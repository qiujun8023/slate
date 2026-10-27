#pragma once

// HTTP client 封装(所有 server 通讯走这里)。默认门面函数委托给 DefaultClient()。
//
// 设备协议 v2(backend/internal/api/device.go),响应统一为 {success, message, data}:
//   POST /api/device/register     注册,换取 secret 与配对码
//   POST /api/device/sync         上报 telemetry,返回 DeviceState;manifestVersion 不同才附带 manifest
//   POST /api/device/group        next|prev|select 切组,返回 DeviceState
//   GET  /api/device/blobs/{hash} 按 sha256 取图片/音频,内容不可变
// 协议版本不一致时服务端回 426。

#include <esp_http_client.h>

#include <atomic>
#include <cstdint>
#include <functional>
#include <mutex>
#include <string>
#include <vector>

struct cJSON;

namespace api {

inline constexpr int kProtocol = 2;

struct Blob {
    std::string hash;  // 空表示没有该资源
    int         size = 0;
};

struct Frame {
    std::string id;
    std::string status_bar_text;
    Blob        image;
    Blob        audio;
    int64_t     refresh_at = 0;  // Unix 秒;0 表示静态帧
};

struct Manifest {
    std::string        version;
    std::vector<Frame> frames;
};

struct DeviceState {
    int64_t server_time_ms = 0;

    std::string id;
    std::string device_name;
    bool        bound = false;
    std::string pair_code;

    bool        has_group = false;
    std::string group_id;
    std::string group_name;
    std::string manifest_version;
    int         content_count    = 0;
    int         position_current = 0;
    int         position_total   = 0;

    // 仅当请求里的 manifestVersion 与服务端不同才下发。
    bool     has_manifest = false;
    Manifest manifest;
};

struct RegisterResult {
    std::string id;
    std::string device_secret;
    std::string pair_code;
    int64_t     server_time_ms = 0;
};

struct Telemetry {
    int         battery_pct = -1;
    int         rssi_dbm    = 0;
    std::string fw_version;
    std::string wake_reason;
    // timer 唤醒时服务端会先重新渲染该内容(若已到期),本轮即可拿到新帧。
    std::string current_content_id;
};

enum class GroupAction { kNext, kPrev, kSelect };

using UnauthorizedCb = std::function<void()>;

class ApiClient {
   public:
    ApiClient() = default;
    ApiClient(std::string server_url, std::string mac, std::string device_secret);

    void Configure(const std::string& server_url, const std::string& mac, const std::string& device_secret);
    void SetServerUrl(const std::string& url);
    void SetSecret(const std::string& secret);
    void SetUnauthorizedHandler(UnauthorizedCb cb);

    // 关闭并释放持久 HTTP 连接(回收 mbedTLS 内存)。在一次 sync 突发结束后调用,
    // 这样连接只在单次突发内(sync/group → 各帧 blob)被复用。
    void ResetConnection();

    bool Register(RegisterResult& out);
    bool Sync(const std::string& manifest_version, const Telemetry& tel, DeviceState& out);
    bool ChangeGroup(GroupAction action, const std::string& group_id, const std::string& manifest_version,
                     DeviceState& out);
    // 下载并校验大小与 sha256。
    bool DownloadBlob(const Blob& blob, std::vector<uint8_t>& out);

   private:
    bool DoRequest(const std::string& path, esp_http_client_method_t method, const std::string& body_in,
                   std::vector<uint8_t>& body_out, bool need_auth, int timeout_ms);
    // 发 JSON 请求并取出 envelope 的 data 对象交给 parse。
    bool PostJson(const std::string& path, const std::string& body, bool need_auth,
                  const std::function<bool(cJSON*)>& parse);

    // 复用 conn_。conn_base_url_ 与请求 base 不一致时重建。调用者需持 conn_mutex_。
    esp_http_client_handle_t EnsureClientLocked(const std::string& base_url);
    // 销毁 conn_(连接进入异常态时调用,强制下次重建)。调用者需持 conn_mutex_。
    void DropConnectionLocked();

    std::string server_url_;
    std::string mac_;
    std::string secret_;

    mutable std::mutex state_mutex_;
    UnauthorizedCb     unauthorized_cb_;
    std::atomic<int>   consecutive_401_{0};
    std::atomic<bool>  warned_http_auth_{false};

    // 持久 HTTP 连接:在一次 sync 突发内复用,避免每请求重做 TLS 握手。
    // conn_mutex_ 串行化整个请求事务并保护 conn_/conn_base_url_。
    std::mutex               conn_mutex_;
    esp_http_client_handle_t conn_ = nullptr;
    std::string              conn_base_url_;
};

ApiClient& DefaultClient();

void Init(const std::string& server_url, const std::string& mac, const std::string& device_secret);
void SetServerUrl(const std::string& url);
void SetSecret(const std::string& secret);
void SetUnauthorizedHandler(UnauthorizedCb cb);
void ResetConnection();

bool Register(RegisterResult& out);
bool Sync(const std::string& manifest_version, const Telemetry& tel, DeviceState& out);
bool ChangeGroup(GroupAction action, const std::string& group_id, const std::string& manifest_version,
                 DeviceState& out);
bool DownloadBlob(const Blob& blob, std::vector<uint8_t>& out);

}  // namespace api
