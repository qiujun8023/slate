#include "storage/cache/cache.h"

#include <esp_littlefs.h>
#include <esp_log.h>

#include "storage/cache/cache_internal.h"
#include "storage/cache/cache_io.h"
#include "storage/cache/cache_paths.h"

namespace cache {
namespace {

// 缓存布局版本。帧元数据、manifest 或 state.json 的格式变化时递增：Init 发现不一致会
// 整体格式化，固件不需要读旧格式。
constexpr char kLayoutVersion[] = "2";

std::string LayoutPath() {
    return std::string(internal::kRoot) + "/layout";
}

bool LayoutCurrent() {
    std::vector<uint8_t> buf;
    return internal::ReadAll(LayoutPath(), buf, 16) && std::string(buf.begin(), buf.end()) == kLayoutVersion;
}

}  // namespace

bool Init() {
    esp_vfs_littlefs_conf_t cfg = {};
    cfg.base_path               = internal::kRoot;
    cfg.partition_label         = "storage";
    cfg.format_if_mount_failed  = true;
    cfg.dont_mount              = false;

    esp_err_t err = esp_vfs_littlefs_register(&cfg);
    if (err != ESP_OK) {
        ESP_LOGE(internal::kTag, "littlefs mount failed err=%s", esp_err_to_name(err));
        return false;
    }
    size_t total = 0, used = 0;
    esp_littlefs_info(cfg.partition_label, &total, &used);
    if (!LayoutCurrent()) {
        ESP_LOGW(internal::kTag, "cache layout mismatch want=%s action=format", kLayoutVersion);
        if (!FormatAll())
            return false;
    }
    internal::DirEnsure(std::string(internal::kRoot) + "/groups");
    return true;
}

bool FormatAll() {
    constexpr char kLabel[] = "storage";
    ESP_LOGW(internal::kTag, "format all action=erase_littlefs_cache");
    esp_err_t err = esp_vfs_littlefs_unregister(kLabel);
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE) {
        ESP_LOGW(internal::kTag, "littlefs unregister failed err=%s action=continue", esp_err_to_name(err));
    }
    err = esp_littlefs_format(kLabel);
    if (err != ESP_OK) {
        ESP_LOGE(internal::kTag, "littlefs format failed err=%s", esp_err_to_name(err));
        return false;
    }

    esp_vfs_littlefs_conf_t cfg = {};
    cfg.base_path               = internal::kRoot;
    cfg.partition_label         = kLabel;
    cfg.format_if_mount_failed  = true;
    cfg.dont_mount              = false;
    err                         = esp_vfs_littlefs_register(&cfg);
    if (err != ESP_OK) {
        ESP_LOGE(internal::kTag, "littlefs remount failed err=%s", esp_err_to_name(err));
        return false;
    }
    internal::ResetStateCache();
    internal::DirEnsure(std::string(internal::kRoot) + "/groups");
    return internal::WriteAll(LayoutPath(), kLayoutVersion, sizeof(kLayoutVersion) - 1);
}

}  // namespace cache
