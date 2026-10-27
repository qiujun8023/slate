#include "storage/cache/cache_paths.h"

#include <cctype>

namespace cache::internal {

std::string SafePathComponent(const std::string& raw) {
    static constexpr char kHex[] = "0123456789ABCDEF";
    std::string           out;
    out.reserve(raw.size() * 3);
    for (unsigned char ch : raw) {
        if (std::isalnum(ch) || ch == '-' || ch == '_') {
            out.push_back(static_cast<char>(ch));
        } else {
            out.push_back('%');
            out.push_back(kHex[ch >> 4]);
            out.push_back(kHex[ch & 0x0F]);
        }
    }
    return out.empty() ? "_" : out;
}

std::string GroupDir(const std::string& gid) {
    return std::string(kRoot) + "/groups/" + SafePathComponent(gid);
}

std::string StatePath() {
    return std::string(kRoot) + "/state.json";
}

std::string FramesDir(const std::string& gid) {
    return GroupDir(gid) + "/frames";
}

std::string StageDir(const std::string& gid) {
    return GroupDir(gid) + "/stage";
}

std::string ImagePath(const std::string& gid, int idx) {
    return FramesDir(gid) + "/" + std::to_string(idx) + ".img";
}

std::string AudioPath(const std::string& gid, int idx) {
    return FramesDir(gid) + "/" + std::to_string(idx) + ".pcm";
}

std::string StageImagePath(const std::string& gid, int idx) {
    return StageDir(gid) + "/" + std::to_string(idx) + ".img";
}

std::string StageAudioPath(const std::string& gid, int idx) {
    return StageDir(gid) + "/" + std::to_string(idx) + ".pcm";
}

std::string ManifestPath(const std::string& gid) {
    return GroupDir(gid) + "/manifest.json";
}

std::string MetaPath(const std::string& gid, int idx) {
    return FramesDir(gid) + "/" + std::to_string(idx) + ".meta";
}

std::string StageMetaPath(const std::string& gid, int idx) {
    return StageDir(gid) + "/" + std::to_string(idx) + ".meta";
}

}  // namespace cache::internal
