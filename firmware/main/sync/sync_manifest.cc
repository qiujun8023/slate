#include "sync/sync_service.h"

#include <esp_log.h>

#include "events/event_bus.h"
#include "power/power_state.h"
#include "sync/sync_internal.h"
#include "utils/time_utils.h"

using sync_internal::ClampProgressCount;
using sync_internal::kCacheMinFreeBytes;
using sync_internal::kMaxCachedGroups;
using sync_internal::kTag;

bool SyncService::HandleCachedManifestHit(const std::string& gid, const std::string& manifest_version,
                                          const std::string& status_name, int content_count,
                                          const std::string& previous_current, const std::string& selected_group_id,
                                          SyncReason reason, bool content_changed) {
    if (!cache::WriteStateMeta(gid, manifest_version))
        return false;
    cache::TouchGroup(gid);
    SetCurrentGroup(gid);
    if (reason == SyncReason::kCycle && previous_current != gid)
        evt::PostGroupSyncStatus(GroupSyncStatusMode::kCycleCacheHit, gid, status_name);
    if (content_changed || previous_current != gid || selected_group_id != gid)
        PostSyncedGroupReady(gid, status_name, content_count, content_changed);
    return true;
}

bool SyncService::DownloadFramesToStage(cache::CacheWriter& writer, const std::string& gid,
                                        const api::Manifest& manifest, const std::string& status_name,
                                        const std::string& previous_current, const std::string& selected_group_id,
                                        SyncReason reason, int& total_updates) {
    total_updates = 0;
    for (int idx = 0; idx < static_cast<int>(manifest.frames.size()); ++idx) {
        const auto& f = manifest.frames[idx];
        if (!writer.FrameImageExists(idx, f.image.hash))
            ++total_updates;
        if (!f.audio.hash.empty() && !writer.FrameAudioExists(idx, f.audio.hash))
            ++total_updates;
    }

    int        done     = 0;
    bool       complete = true;
    const bool current_group_update =
        reason != SyncReason::kCycle && (previous_current == gid || selected_group_id == gid);
    const GroupSyncStatusMode progress_mode = reason == SyncReason::kCycle ? GroupSyncStatusMode::kCycleDownloading
                                              : current_group_update ? GroupSyncStatusMode::kCurrentGroupUpdating
                                                                     : GroupSyncStatusMode::kInitialGroupDownloading;
    auto                      post_progress = [&]() {
        const uint8_t cur = ClampProgressCount(done);
        const uint8_t all = ClampProgressCount(total_updates);
        evt::PostGroupSyncStatus(progress_mode, gid, status_name, cur, all);
        evt::PostSyncProgress(cur, all);
    };
    if (total_updates > 0)
        post_progress();

    auto download = [&](int idx, const api::Blob& blob, const char* kind, bool image) {
        const int64_t started_ms = time_utils::NowMs();
        download_buf_.clear();
        if (!api::DownloadBlob(blob, download_buf_)) {
            ESP_LOGW(kTag, "frame %s download failed idx=%d elapsed_ms=%lld", kind, idx,
                     (long long)(time_utils::NowMs() - started_ms));
            return false;
        }
        const bool written = image ? writer.WriteFrameImage(idx, download_buf_, blob.hash)
                                   : writer.WriteFrameAudio(idx, download_buf_, blob.hash);
        if (!written)
            ESP_LOGW(kTag, "frame %s write failed idx=%d", kind, idx);
        return written;
    };

    for (int idx = 0; idx < static_cast<int>(manifest.frames.size()); ++idx) {
        const auto& f = manifest.frames[idx];
        if (ShouldStop())
            return false;
        if (!writer.FrameImageExists(idx, f.image.hash)) {
            complete = download(idx, f.image, "image", true) && complete;
            ++done;
            post_progress();
        }
        if (ShouldStop())
            return false;
        if (!f.audio.hash.empty() && !writer.FrameAudioExists(idx, f.audio.hash)) {
            complete = download(idx, f.audio, "audio", false) && complete;
            ++done;
            post_progress();
        }

        if (writer.FrameImageExists(idx, f.image.hash) &&
            (f.audio.hash.empty() || writer.FrameAudioExists(idx, f.audio.hash))) {
            cache::FrameMeta fm;
            fm.status_bar_text = f.status_bar_text;
            fm.content_id      = f.id;
            fm.image_hash      = f.image.hash;
            fm.audio_hash      = f.audio.hash;
            fm.refresh_at      = f.refresh_at;
            if (!writer.WriteFrameMeta(idx, fm)) {
                ESP_LOGW(kTag, "frame meta write failed idx=%d", idx);
                complete = false;
            }
        } else {
            complete = false;
        }
    }
    return complete;
}

bool SyncService::CommitStagedFrames(cache::CacheWriter& writer, const std::string& gid, const api::Manifest& manifest,
                                     const std::string& group_name, const std::string& selected_group_id,
                                     bool current_group_update, SyncReason reason, int total_updates,
                                     int old_content_count) {
    const GroupSyncStatusMode saving_mode =
        current_group_update ? GroupSyncStatusMode::kCurrentGroupSaving : GroupSyncStatusMode::kTargetGroupSaving;
    const int total = static_cast<int>(manifest.frames.size());
    if (total > 0)
        evt::PostGroupSyncStatus(saving_mode, gid, group_name, 0, ClampProgressCount(total));
    for (int idx = 0; idx < total; ++idx) {
        const auto& f = manifest.frames[idx];
        if (!writer.CommitFrame(idx, f.image.hash, f.audio.hash)) {
            ESP_LOGW(kTag, "frame commit failed idx=%d", idx);
            writer.Rollback();
            return false;
        }
        evt::PostGroupSyncStatus(saving_mode, gid, group_name, ClampProgressCount(idx + 1), ClampProgressCount(total));
    }
    if (!cache::WriteManifest(gid, manifest.version, total, group_name)) {
        ESP_LOGW(kTag, "manifest write failed action=rollback");
        writer.Rollback();
        return false;
    }
    if (!cache::WriteStateMeta(gid, manifest.version)) {
        ESP_LOGW(kTag, "state write failed action=rollback");
        writer.Rollback();
        return false;
    }
    cache::TouchGroup(gid);
    cache::PruneOldGroups(selected_group_id, gid, kCacheMinFreeBytes, kMaxCachedGroups);
    for (int idx = 0; idx < total; ++idx) {
        if (manifest.frames[idx].audio.hash.empty())
            cache::DeleteFrameAudio(gid, idx);
    }
    for (int idx = total; idx < old_content_count; ++idx) {
        cache::DeleteFrameFiles(gid, idx);
    }
    writer.Commit();
    SetCurrentGroup(gid);
    if (reason == SyncReason::kCycle && total_updates == 0)
        evt::PostGroupSyncStatus(GroupSyncStatusMode::kCycleCacheHit, gid, group_name);
    PostSyncedGroupReady(gid, group_name, total, /*content_changed=*/true);
    return true;
}

bool SyncService::SyncManifestAndFrames(const api::DeviceState& state, SyncReason reason, bool& group_changed) {
    group_changed         = false;
    const std::string gid = state.group_id;

    const std::string previous_current = CurrentGroupSnapshot();
    std::string       selected_group_id, selected_version;
    cache::ReadStateMeta(selected_group_id, selected_version);

    cache::ManifestMeta cached_meta;
    bool                cached_meta_ok = cache::ReadManifestMeta(gid, cached_meta);
    if (cached_meta_ok && !state.group_name.empty() && cached_meta.name != state.group_name) {
        cache::WriteManifest(gid, cached_meta.manifest_version, cached_meta.content_count, state.group_name);
        cached_meta_ok = cache::ReadManifestMeta(gid, cached_meta);
    }
    const std::string status_name = !state.group_name.empty() ? state.group_name : cached_meta.name;

    // 上次提交中途失败时屏幕可能还是旧画面,这一轮成功后无论缓存是否命中都要重绘。
    const bool commit_interrupted = power_state::CacheCommitInterrupted();
    if (cached_meta_ok && cached_meta.manifest_version == state.manifest_version) {
        if (!HandleCachedManifestHit(gid, state.manifest_version, status_name, state.content_count, previous_current,
                                     selected_group_id, reason, commit_interrupted))
            return false;
        if (commit_interrupted) {
            power_state::SetCacheCommitInterrupted(false);
            group_changed = true;
        }
        return true;
    }
    if (!state.has_manifest) {
        // 服务端认为设备已有该版本,但本地缓存对不上(缓存损坏或被清理)。清掉版本号,
        // 下一轮 sync 会拿到完整 manifest。
        ESP_LOGW(kTag, "manifest missing reason=cache_mismatch action=request_full_next_sync");
        cache::WriteStateMeta(selected_group_id, "");
        return false;
    }
    if (ShouldStop())
        return false;

    // 后台刷新只在当前展示帧的像素或状态栏变化时重绘,refresh_at 变化只更新调度。
    const bool       same_group = selected_group_id == gid;
    const int        cur_seq    = power_state::GetCurrentFrameSeq();
    cache::FrameMeta old_current;
    const bool       old_current_ok = same_group && cache::ReadFrameMeta(gid, cur_seq, old_current);

    int old_content_count = 0;
    cache::ReadManifestContentCount(gid, old_content_count);
    cache::CacheWriter writer(gid);
    if (!writer.Begin()) {
        ESP_LOGW(kTag, "frame stage init failed");
        return false;
    }

    const api::Manifest& mf            = state.manifest;
    int                  total_updates = 0;
    if (!DownloadFramesToStage(writer, gid, mf, status_name, previous_current, selected_group_id, reason,
                               total_updates)) {
        ESP_LOGW(kTag, "manifest sync incomplete action=rollback");
        writer.Rollback();
        return false;
    }
    if (ShouldStop()) {
        writer.Rollback();
        return false;
    }

    const bool current_group_update =
        reason != SyncReason::kCycle && (previous_current == gid || selected_group_id == gid);
    power_state::SetCacheCommitInterrupted(true);
    if (!CommitStagedFrames(writer, gid, mf, status_name, selected_group_id, current_group_update, reason,
                            total_updates, old_content_count)) {
        return false;
    }
    power_state::SetCacheCommitInterrupted(false);

    group_changed = true;
    if (same_group && cur_seq < static_cast<int>(mf.frames.size())) {
        cache::FrameMeta now_current;
        if (cache::ReadFrameMeta(gid, cur_seq, now_current)) {
            power_state::SetCurrentFrameFromMeta(cur_seq, now_current);
            if (reason == SyncReason::kBackgroundRefresh && old_current_ok && !commit_interrupted)
                group_changed = old_current.image_hash != now_current.image_hash ||
                                old_current.status_bar_text != now_current.status_bar_text;
        }
    }
    return true;
}
