#pragma once

#include <string>

namespace cache::internal {

inline constexpr char kRoot[] = "/littlefs";

std::string SafePathComponent(const std::string& raw);
std::string GroupDir(const std::string& gid);
std::string StatePath();
std::string FramesDir(const std::string& gid);
std::string StageDir(const std::string& gid);
std::string ImagePath(const std::string& gid, int idx);
std::string AudioPath(const std::string& gid, int idx);
std::string StageImagePath(const std::string& gid, int idx);
std::string StageAudioPath(const std::string& gid, int idx);
std::string ManifestPath(const std::string& gid);
std::string MetaPath(const std::string& gid, int idx);
std::string StageMetaPath(const std::string& gid, int idx);

}  // namespace cache::internal
