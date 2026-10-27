#include "storage/cache/cache.h"

#include <cJSON.h>
#include <esp_log.h>
#include <sys/stat.h>

#include <unistd.h>
#include <cstdint>
#include <cstring>
#include <utility>

#include "storage/cache/cache_io.h"
#include "storage/cache/cache_paths.h"
#include "storage/cache/cache_staging.h"

namespace {

bool WriteFrameMetaFile(const std::string& path, const cache::FrameMeta& meta) {
    cJSON* root = cJSON_CreateObject();
    if (!root)
        return false;
    cJSON_AddStringToObject(root, "status_bar_text", meta.status_bar_text.c_str());
    cJSON_AddStringToObject(root, "content_id", meta.content_id.c_str());
    cJSON_AddStringToObject(root, "image_hash", meta.image_hash.c_str());
    cJSON_AddStringToObject(root, "audio_hash", meta.audio_hash.c_str());
    cJSON_AddNumberToObject(root, "refresh_at", static_cast<double>(meta.refresh_at));
    char* s = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    if (!s)
        return false;
    const size_t len = std::strlen(s);
    const bool   ok  = cache::internal::WriteAll(path, s, len);
    cJSON_free(s);
    return ok;
}

bool ReadFrameMetaFile(const std::string& path, cache::FrameMeta& out) {
    out = {};
    std::vector<uint8_t> buf;
    if (!cache::internal::ReadAll(path, buf, cache::internal::kMaxFrameMetaBytes) || buf.empty())
        return false;
    cJSON* root = cJSON_ParseWithLength(reinterpret_cast<const char*>(buf.data()), buf.size());
    if (!root)
        return false;
    cJSON* cap        = cJSON_GetObjectItemCaseSensitive(root, "status_bar_text");
    cJSON* content_id = cJSON_GetObjectItemCaseSensitive(root, "content_id");
    cJSON* image_hash = cJSON_GetObjectItemCaseSensitive(root, "image_hash");
    cJSON* audio_hash = cJSON_GetObjectItemCaseSensitive(root, "audio_hash");
    cJSON* refresh_at = cJSON_GetObjectItemCaseSensitive(root, "refresh_at");
    if (cJSON_IsString(cap) && cap->valuestring)
        out.status_bar_text = cap->valuestring;
    if (cJSON_IsString(content_id) && content_id->valuestring)
        out.content_id = content_id->valuestring;
    if (cJSON_IsString(image_hash) && image_hash->valuestring)
        out.image_hash = image_hash->valuestring;
    if (cJSON_IsString(audio_hash) && audio_hash->valuestring)
        out.audio_hash = audio_hash->valuestring;
    if (cJSON_IsNumber(refresh_at) && refresh_at->valuedouble > 0.0)
        out.refresh_at = static_cast<int64_t>(refresh_at->valuedouble);
    cJSON_Delete(root);
    return true;
}

bool UpdateStagedFrameHashes(const std::string& gid, int idx, const std::string& image_hash,
                             const std::string& audio_hash) {
    cache::FrameMeta meta;
    ReadFrameMetaFile(cache::internal::StageMetaPath(gid, idx), meta);
    if (!image_hash.empty())
        meta.image_hash = image_hash;
    if (!audio_hash.empty())
        meta.audio_hash = audio_hash;
    return WriteFrameMetaFile(cache::internal::StageMetaPath(gid, idx), meta);
}

}  // namespace

namespace cache {

bool FrameImageExists(const std::string& gid, int idx, const std::string& expected_hash) {
    if (expected_hash.empty())
        return false;
    struct stat st;
    if (stat(internal::ImagePath(gid, idx).c_str(), &st) != 0)
        return false;
    if (st.st_size != internal::kFrameImageBytes)
        return false;
    FrameMeta meta;
    return ReadFrameMeta(gid, idx, meta) && meta.image_hash == expected_hash;
}

bool WriteFrameImage(const std::string& gid, int idx, const std::vector<uint8_t>& bytes, const std::string& hash) {
    (void)hash;
    if (bytes.size() != internal::kFrameImageBytes) {
        ESP_LOGW(internal::kTag, "frame image refused idx=%d bytes=%u expected=%u", idx,
                 static_cast<unsigned>(bytes.size()), static_cast<unsigned>(internal::kFrameImageBytes));
        return false;
    }
    internal::DirEnsure(internal::GroupDir(gid));
    internal::DirEnsure(internal::FramesDir(gid));
    return internal::WriteAll(internal::ImagePath(gid, idx), bytes.data(), bytes.size());
}

bool ReadFrameImage(const std::string& gid, int idx, std::vector<uint8_t>& out) {
    return internal::ReadAll(internal::ImagePath(gid, idx), out, internal::kFrameImageBytes);
}

bool FrameAudioExists(const std::string& gid, int idx, const std::string& expected_hash) {
    if (expected_hash.empty())
        return false;
    struct stat st;
    if (stat(internal::AudioPath(gid, idx).c_str(), &st) != 0)
        return false;
    FrameMeta meta;
    return ReadFrameMeta(gid, idx, meta) && meta.audio_hash == expected_hash;
}

bool WriteFrameAudio(const std::string& gid, int idx, const std::vector<uint8_t>& bytes, const std::string& hash) {
    (void)hash;
    internal::DirEnsure(internal::GroupDir(gid));
    internal::DirEnsure(internal::FramesDir(gid));
    return internal::WriteAll(internal::AudioPath(gid, idx), bytes.data(), bytes.size());
}

bool ReadFrameAudio(const std::string& gid, int idx, std::vector<uint8_t>& out) {
    return internal::ReadAll(internal::AudioPath(gid, idx), out, internal::kMaxAudioReadBytes);
}

void DeleteFrameAudio(const std::string& gid, int idx) {
    unlink(internal::AudioPath(gid, idx).c_str());
}

void DeleteFrameFiles(const std::string& gid, int idx) {
    unlink(internal::ImagePath(gid, idx).c_str());
    DeleteFrameAudio(gid, idx);
    unlink(internal::MetaPath(gid, idx).c_str());
}

bool WriteFrameMeta(const std::string& gid, int idx, const FrameMeta& meta) {
    internal::DirEnsure(internal::GroupDir(gid));
    internal::DirEnsure(internal::FramesDir(gid));
    return WriteFrameMetaFile(internal::MetaPath(gid, idx), meta);
}

bool ReadFrameMeta(const std::string& gid, int idx, FrameMeta& out) {
    return ReadFrameMetaFile(internal::MetaPath(gid, idx), out);
}

namespace {
bool BeginFrameStage(const std::string& gid) {
    if (gid.empty())
        return false;
    if (!internal::RemoveTree(internal::StageDir(gid)))
        return false;
    internal::DirEnsure(std::string(internal::kRoot) + "/groups");
    internal::DirEnsure(internal::GroupDir(gid));
    return internal::DirEnsure(internal::StageDir(gid));
}

void CleanupFrameStage(const std::string& gid) {
    if (!gid.empty())
        internal::RemoveTree(internal::StageDir(gid));
}

bool StagedFrameImageExists(const std::string& gid, int idx, const std::string& expected_hash) {
    if (expected_hash.empty())
        return false;
    struct stat st;
    if (stat(internal::StageImagePath(gid, idx).c_str(), &st) == 0 && st.st_size == internal::kFrameImageBytes) {
        FrameMeta meta;
        if (ReadFrameMetaFile(internal::StageMetaPath(gid, idx), meta) && meta.image_hash == expected_hash)
            return true;
    }
    return FrameImageExists(gid, idx, expected_hash);
}

bool WriteStagedFrameImage(const std::string& gid, int idx, const std::vector<uint8_t>& bytes,
                           const std::string& hash) {
    if (bytes.size() != internal::kFrameImageBytes) {
        ESP_LOGW(internal::kTag, "staged frame image refused idx=%d bytes=%u expected=%u", idx,
                 static_cast<unsigned>(bytes.size()), static_cast<unsigned>(internal::kFrameImageBytes));
        return false;
    }
    internal::DirEnsure(internal::StageDir(gid));
    if (!internal::WriteAll(internal::StageImagePath(gid, idx), bytes.data(), bytes.size()))
        return false;
    return UpdateStagedFrameHashes(gid, idx, hash, "");
}

bool StagedFrameAudioExists(const std::string& gid, int idx, const std::string& expected_hash) {
    if (expected_hash.empty())
        return false;
    struct stat st;
    if (stat(internal::StageAudioPath(gid, idx).c_str(), &st) == 0) {
        FrameMeta meta;
        if (ReadFrameMetaFile(internal::StageMetaPath(gid, idx), meta) && meta.audio_hash == expected_hash)
            return true;
    }
    return FrameAudioExists(gid, idx, expected_hash);
}

bool WriteStagedFrameAudio(const std::string& gid, int idx, const std::vector<uint8_t>& bytes,
                           const std::string& hash) {
    internal::DirEnsure(internal::StageDir(gid));
    if (!internal::WriteAll(internal::StageAudioPath(gid, idx), bytes.data(), bytes.size()))
        return false;
    return UpdateStagedFrameHashes(gid, idx, "", hash);
}

bool WriteStagedFrameMeta(const std::string& gid, int idx, const FrameMeta& meta) {
    internal::DirEnsure(internal::StageDir(gid));
    return WriteFrameMetaFile(internal::StageMetaPath(gid, idx), meta);
}

bool CommitStagedFrame(const std::string& gid, int idx, const std::string& image_hash, const std::string& audio_hash) {
    internal::DirEnsure(internal::GroupDir(gid));
    internal::DirEnsure(internal::FramesDir(gid));

    const std::string staged_image = internal::StageImagePath(gid, idx);
    const std::string staged_meta  = internal::StageMetaPath(gid, idx);
    const std::string staged_audio = internal::StageAudioPath(gid, idx);
    FrameMeta         staged_frame_meta;
    if (!ReadFrameMetaFile(staged_meta, staged_frame_meta)) {
        ESP_LOGW(internal::kTag, "staged meta missing idx=%d", idx);
        return false;
    }
    if (staged_frame_meta.image_hash != image_hash) {
        ESP_LOGW(internal::kTag, "staged image hash mismatch idx=%d", idx);
        return false;
    }
    if (!internal::PathExists(staged_image) && !FrameImageExists(gid, idx, image_hash)) {
        ESP_LOGW(internal::kTag, "frame image missing idx=%d", idx);
        return false;
    }
    if (!audio_hash.empty()) {
        if (staged_frame_meta.audio_hash != audio_hash) {
            ESP_LOGW(internal::kTag, "staged audio hash mismatch idx=%d", idx);
            return false;
        }
        if (!internal::PathExists(staged_audio) && !FrameAudioExists(gid, idx, audio_hash)) {
            ESP_LOGW(internal::kTag, "frame audio missing idx=%d", idx);
            return false;
        }
    }

    std::vector<staging::Swap> swaps;
    if (internal::PathExists(staged_image)) {
        swaps.push_back({staged_image, internal::ImagePath(gid, idx), internal::ImagePath(gid, idx) + ".bak"});
    }
    if (!audio_hash.empty() && internal::PathExists(staged_audio)) {
        swaps.push_back({staged_audio, internal::AudioPath(gid, idx), internal::AudioPath(gid, idx) + ".bak"});
    }
    swaps.push_back({staged_meta, internal::MetaPath(gid, idx), internal::MetaPath(gid, idx) + ".bak"});

    return staging::CommitSwaps(swaps);
}
}  // namespace

CacheWriter::CacheWriter(std::string gid) : gid_(std::move(gid)) {
}

CacheWriter::~CacheWriter() {
    Rollback();
}

bool CacheWriter::Begin() {
    if (begun_)
        return true;
    begun_ = BeginFrameStage(gid_);
    return begun_;
}

bool CacheWriter::FrameImageExists(int idx, const std::string& expected_hash) const {
    return begun_ && StagedFrameImageExists(gid_, idx, expected_hash);
}

bool CacheWriter::WriteFrameImage(int idx, const std::vector<uint8_t>& bytes, const std::string& hash) {
    return begun_ && WriteStagedFrameImage(gid_, idx, bytes, hash);
}

bool CacheWriter::FrameAudioExists(int idx, const std::string& expected_hash) const {
    return begun_ && StagedFrameAudioExists(gid_, idx, expected_hash);
}

bool CacheWriter::WriteFrameAudio(int idx, const std::vector<uint8_t>& bytes, const std::string& hash) {
    return begun_ && WriteStagedFrameAudio(gid_, idx, bytes, hash);
}

bool CacheWriter::WriteFrameMeta(int idx, const FrameMeta& meta) {
    return begun_ && WriteStagedFrameMeta(gid_, idx, meta);
}

bool CacheWriter::CommitFrame(int idx, const std::string& image_hash, const std::string& audio_hash) {
    return begun_ && CommitStagedFrame(gid_, idx, image_hash, audio_hash);
}

bool CacheWriter::Commit() {
    if (!begun_)
        return false;
    committed_ = true;
    CleanupFrameStage(gid_);
    return true;
}

void CacheWriter::Rollback() {
    if (begun_ && !committed_)
        CleanupFrameStage(gid_);
    begun_ = false;
}

}  // namespace cache
