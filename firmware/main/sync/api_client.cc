#include "sync/api_client.h"

#include <cJSON.h>
#include <esp_crt_bundle.h>
#include <esp_http_client.h>
#include <esp_log.h>
#include <mbedtls/sha256.h>

#include <atomic>
#include <cstdio>
#include <cstring>
#include <mutex>
#include <utility>

#include "bsp/config.h"
#include "utils/json_utils.h"
#include "utils/time_utils.h"

namespace {
constexpr char kTag[]                      = "api";
constexpr int  kUnauthorizedResetThreshold = 5;
constexpr int  kDefaultTimeoutMs           = 8000;
// sync/group 可能附带 manifest,timer 唤醒时服务端还会先渲染当前帧。
constexpr int kSyncTimeoutMs = 25000;
constexpr int kBlobTimeoutMs = 20000;

namespace proto {

inline constexpr char kAction[]           = "action";
inline constexpr char kAudio[]            = "audio";
inline constexpr char kBatteryPct[]       = "batteryPct";
inline constexpr char kBound[]            = "bound";
inline constexpr char kContentCount[]     = "contentCount";
inline constexpr char kCurrent[]          = "current";
inline constexpr char kCurrentContentId[] = "currentContentId";
inline constexpr char kData[]             = "data";
inline constexpr char kDevice[]           = "device";
inline constexpr char kDeviceId[]         = "deviceId";
inline constexpr char kFrames[]           = "frames";
inline constexpr char kFwVersion[]        = "fwVersion";
inline constexpr char kGroup[]            = "group";
inline constexpr char kGroupId[]          = "groupId";
inline constexpr char kHash[]             = "hash";
inline constexpr char kId[]               = "id";
inline constexpr char kImage[]            = "image";
inline constexpr char kMac[]              = "mac";
inline constexpr char kManifest[]         = "manifest";
inline constexpr char kManifestVersion[]  = "manifestVersion";
inline constexpr char kMessage[]          = "message";
inline constexpr char kName[]             = "name";
inline constexpr char kPairCode[]         = "pairCode";
inline constexpr char kPosition[]         = "position";
inline constexpr char kProtocol[]         = "protocol";
inline constexpr char kRefreshAt[]        = "refreshAt";
inline constexpr char kRssiDbm[]          = "rssiDbm";
inline constexpr char kSecret[]           = "secret";
inline constexpr char kServerTime[]       = "serverTime";
inline constexpr char kSize[]             = "size";
inline constexpr char kStatusBarText[]    = "statusBarText";
inline constexpr char kSuccess[]          = "success";
inline constexpr char kTelemetry[]        = "telemetry";
inline constexpr char kTotal[]            = "total";
inline constexpr char kVersion[]          = "version";
inline constexpr char kWakeReason[]       = "wakeReason";

}  // namespace proto

bool IsAllowedBaseUrl(const std::string& url) {
    if (url.empty() || url.size() > 256)
        return false;
    for (unsigned char ch : url) {
        if (ch <= 0x20 || ch == 0x7F)
            return false;
    }
    return url.rfind("https://", 0) == 0 || url.rfind("http://", 0) == 0;
}

}  // namespace

namespace api {

ApiClient::ApiClient(std::string server_url, std::string mac, std::string device_secret)
    : server_url_(std::move(server_url)), mac_(std::move(mac)), secret_(std::move(device_secret)) {
}

void ApiClient::Configure(const std::string& server_url, const std::string& mac, const std::string& device_secret) {
    std::lock_guard<std::mutex> lock(state_mutex_);
    server_url_ = server_url;
    mac_        = mac;
    secret_     = device_secret;
}

void ApiClient::SetServerUrl(const std::string& url) {
    {
        std::lock_guard<std::mutex> lock(state_mutex_);
        server_url_ = url;
    }
    // host 可能变了,丢弃旧连接。锁顺序:此处不持 state_mutex_ 再取 conn_mutex_。
    ResetConnection();
}

void ApiClient::SetSecret(const std::string& secret) {
    std::lock_guard<std::mutex> lock(state_mutex_);
    secret_ = secret;
}

void ApiClient::SetUnauthorizedHandler(UnauthorizedCb cb) {
    std::lock_guard<std::mutex> lock(state_mutex_);
    unauthorized_cb_ = std::move(cb);
}

namespace {

using json_utils::JsonBool;
using json_utils::JsonInt;
using json_utils::JsonString;

int64_t JsonInt64(const cJSON* obj, const char* key) {
    cJSON* item = cJSON_GetObjectItemCaseSensitive(obj, key);
    return cJSON_IsNumber(item) ? static_cast<int64_t>(item->valuedouble) : 0;
}

void LogErrorEnvelope(const std::string& path, int status, const std::vector<uint8_t>& body) {
    if (body.empty()) {
        ESP_LOGW(kTag, "http error path=%s status=%d", path.c_str(), status);
        return;
    }
    cJSON* root = cJSON_ParseWithLength(reinterpret_cast<const char*>(body.data()), body.size());
    if (!root) {
        ESP_LOGW(kTag, "http error path=%s status=%d body=json_invalid bytes=%u", path.c_str(), status,
                 (unsigned)body.size());
        return;
    }
    ESP_LOGW(kTag, "http error path=%s status=%d message=%s", path.c_str(), status,
             JsonString(root, proto::kMessage).c_str());
    cJSON_Delete(root);
}

bool IsBlobHash(const std::string& hash) {
    if (hash.size() != 64)
        return false;
    for (char ch : hash) {
        if (!((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')))
            return false;
    }
    return true;
}

bool ParseBlob(cJSON* obj, Blob& out) {
    out = {};
    if (!cJSON_IsObject(obj))
        return false;
    out.hash = JsonString(obj, proto::kHash);
    out.size = JsonInt(obj, proto::kSize, 0);
    return IsBlobHash(out.hash) && out.size > 0;
}

bool ParseManifest(cJSON* obj, Manifest& out) {
    out         = {};
    out.version = JsonString(obj, proto::kVersion);
    if (out.version.empty()) {
        ESP_LOGW(kTag, "manifest invalid reason=version_missing");
        return false;
    }
    cJSON* frames = cJSON_GetObjectItemCaseSensitive(obj, proto::kFrames);
    if (!cJSON_IsArray(frames)) {
        ESP_LOGW(kTag, "manifest invalid reason=frames_missing");
        return false;
    }
    cJSON* item = nullptr;
    cJSON_ArrayForEach(item, frames) {
        Frame f;
        f.id              = JsonString(item, proto::kId);
        f.status_bar_text = JsonString(item, proto::kStatusBarText);
        f.refresh_at      = JsonInt64(item, proto::kRefreshAt);
        if (f.id.empty() || !ParseBlob(cJSON_GetObjectItemCaseSensitive(item, proto::kImage), f.image)) {
            ESP_LOGW(kTag, "manifest invalid reason=frame_image index=%u", (unsigned)out.frames.size());
            return false;
        }
        cJSON* audio = cJSON_GetObjectItemCaseSensitive(item, proto::kAudio);
        if (cJSON_IsObject(audio) && !ParseBlob(audio, f.audio)) {
            ESP_LOGW(kTag, "manifest invalid reason=frame_audio index=%u", (unsigned)out.frames.size());
            return false;
        }
        out.frames.push_back(std::move(f));
    }
    return true;
}

bool ParseDeviceState(cJSON* data, DeviceState& out) {
    out                = {};
    out.server_time_ms = JsonInt64(data, proto::kServerTime);

    cJSON* dev = cJSON_GetObjectItemCaseSensitive(data, proto::kDevice);
    if (!cJSON_IsObject(dev)) {
        ESP_LOGW(kTag, "device state invalid reason=device_missing");
        return false;
    }
    out.id          = JsonString(dev, proto::kId);
    out.device_name = JsonString(dev, proto::kName);
    out.bound       = JsonBool(dev, proto::kBound, false);
    out.pair_code   = JsonString(dev, proto::kPairCode);

    cJSON* group = cJSON_GetObjectItemCaseSensitive(data, proto::kGroup);
    if (cJSON_IsObject(group)) {
        out.has_group        = true;
        out.group_id         = JsonString(group, proto::kId);
        out.group_name       = JsonString(group, proto::kName);
        out.manifest_version = JsonString(group, proto::kManifestVersion);
        out.content_count    = JsonInt(group, proto::kContentCount, 0);
        if (out.group_id.empty() || out.manifest_version.empty()) {
            ESP_LOGW(kTag, "device state invalid reason=group_fields_missing");
            return false;
        }
        cJSON* pos = cJSON_GetObjectItemCaseSensitive(group, proto::kPosition);
        if (cJSON_IsObject(pos)) {
            out.position_current = JsonInt(pos, proto::kCurrent, 0);
            out.position_total   = JsonInt(pos, proto::kTotal, 0);
        }
    }

    cJSON* manifest = cJSON_GetObjectItemCaseSensitive(data, proto::kManifest);
    if (cJSON_IsObject(manifest)) {
        if (!out.has_group || !ParseManifest(manifest, out.manifest))
            return false;
        if (out.manifest.version != out.manifest_version) {
            ESP_LOGW(kTag, "device state invalid reason=manifest_version_mismatch");
            return false;
        }
        out.has_manifest = true;
    }
    return true;
}

// 构造 {protocol: 2, ...};返回 nullptr 表示内存不足。
cJSON* NewProtocolBody() {
    cJSON* root = cJSON_CreateObject();
    if (root)
        cJSON_AddNumberToObject(root, proto::kProtocol, kProtocol);
    return root;
}

std::string PrintBody(cJSON* root) {
    if (!root)
        return "";
    return json_utils::PrintAndDelete(root, "");
}

}  // namespace

// 取得可复用的持久 client。base_url 变化(换 host)时重建。调用者持 conn_mutex_。
esp_http_client_handle_t ApiClient::EnsureClientLocked(const std::string& base_url) {
    if (conn_ && conn_base_url_ != base_url) {
        esp_http_client_cleanup(conn_);
        conn_ = nullptr;
        conn_base_url_.clear();
    }
    if (!conn_) {
        esp_http_client_config_t cfg = {};
        cfg.url                      = base_url.c_str();
        cfg.timeout_ms               = kDefaultTimeoutMs;
        cfg.disable_auto_redirect    = false;
        // 挂 IDF 内置 root CA bundle:HTTPS 必备(否则 TLS 握手无 CA 校验失败)。
        // 注意调用方需要确保系统时间已对时(SNTP),否则证书 NotBefore/NotAfter 校验过不去。
        cfg.crt_bundle_attach = esp_crt_bundle_attach;
        // keep-alive:同一 host 的多次请求复用 TCP/TLS,免去每请求重做握手。
        cfg.keep_alive_enable = true;
        conn_                 = esp_http_client_init(&cfg);
        if (conn_)
            conn_base_url_ = base_url;
    }
    return conn_;
}

// 销毁持久连接(进入异常态、或主动回收内存时)。调用者持 conn_mutex_。
void ApiClient::DropConnectionLocked() {
    if (conn_) {
        esp_http_client_cleanup(conn_);
        conn_ = nullptr;
        conn_base_url_.clear();
    }
}

void ApiClient::ResetConnection() {
    std::lock_guard<std::mutex> lock(conn_mutex_);
    DropConnectionLocked();
}

// 同步 HTTP 请求,只有 2xx 返回 true。
//   need_auth - true:加 Authorization: Bearer secret_。register 端点设 false。
//
// 401:仅对 need_auth=true 的请求计数,连续 kUnauthorizedResetThreshold 次触发 unauth_cb。
bool ApiClient::DoRequest(const std::string& path, esp_http_client_method_t method, const std::string& body_in,
                          std::vector<uint8_t>& body_out, bool need_auth, int timeout_ms) {
    const int64_t started_ms = time_utils::NowMs();
    std::string   base_url, secret;
    {
        std::lock_guard<std::mutex> lock(state_mutex_);
        base_url = server_url_;
        secret   = secret_;
    }
    if (base_url.empty()) {
        ESP_LOGW(kTag, "request failed reason=server_url_missing");
        return false;
    }
    if (!IsAllowedBaseUrl(base_url)) {
        ESP_LOGW(kTag, "request failed reason=server_url_rejected");
        return false;
    }
    if (need_auth && secret.empty()) {
        ESP_LOGW(kTag, "request failed reason=secret_empty path=%s", path.c_str());
        return false;
    }
    if (need_auth && base_url.rfind("http://", 0) == 0 &&
        !warned_http_auth_.exchange(true, std::memory_order_relaxed)) {
        ESP_LOGW(kTag, "request insecure path=%s auth=1 scheme=http", path.c_str());
    }
    std::string normalized_base = base_url;
    if (!normalized_base.empty() && normalized_base.back() == '/')
        normalized_base.pop_back();
    const std::string full = normalized_base + path;

    // 防止异常响应耗尽堆内存。上限与后端音频转码 60 秒 PCM 上限对齐；
    // JSON API 与 1bpp 图片远小于这个值，音频资源也不会被合法 TTS 误拦截。
    constexpr size_t kMaxResponseBytes = static_cast<size_t>(AUDIO_MAX_PCM_BYTES);

    // 整个请求事务串行化:持久 conn_ 不能并发使用。
    std::lock_guard<std::mutex> conn_lock(conn_mutex_);

    // 最多两次尝试:复用的 keep-alive socket 可能已被对端关闭,open 失败时重建重试一次。
    for (int attempt = 0; attempt < 2; ++attempt) {
        const bool               reused = (conn_ != nullptr);
        esp_http_client_handle_t client = EnsureClientLocked(normalized_base);
        if (!client) {
            ESP_LOGW(kTag, "http init failed path=%s elapsed_ms=%lld", path.c_str(),
                     (long long)(time_utils::NowMs() - started_ms));
            return false;
        }

        esp_http_client_set_url(client, full.c_str());
        esp_http_client_set_method(client, method);
        esp_http_client_set_timeout_ms(client, timeout_ms);

        // 复用 handle 时上次请求设置的 header 会残留,先逐个清掉再按需设置。
        esp_http_client_delete_header(client, "Authorization");
        esp_http_client_delete_header(client, "Content-Type");
        if (need_auth) {
            const std::string bearer = "Bearer " + secret;
            esp_http_client_set_header(client, "Authorization", bearer.c_str());
        }
        if (!body_in.empty())
            esp_http_client_set_header(client, "Content-Type", "application/json");

        // open 会建立连接并发送请求头。复用的死 socket 在此暴露,可安全重试(请求体尚未发出)。
        esp_err_t err = esp_http_client_open(client, body_in.size());
        if (err != ESP_OK) {
            ESP_LOGW(kTag, "http open failed path=%s elapsed_ms=%lld err=%s", path.c_str(),
                     (long long)(time_utils::NowMs() - started_ms), esp_err_to_name(err));
            DropConnectionLocked();
            if (reused && attempt == 0)
                continue;  // 可能是过期 keep-alive socket,用新连接重试
            return false;
        }
        if (!body_in.empty()) {
            int wn = esp_http_client_write(client, body_in.c_str(), body_in.size());
            if (wn != static_cast<int>(body_in.size())) {
                ESP_LOGW(kTag, "http write short path=%s elapsed_ms=%lld written=%d expected=%u", path.c_str(),
                         (long long)(time_utils::NowMs() - started_ms), wn, (unsigned)body_in.size());
                DropConnectionLocked();
                return false;  // 请求头已发出,不重试(POST 非幂等)
            }
        }

        int64_t content_length = esp_http_client_fetch_headers(client);
        if (content_length < 0) {
            ESP_LOGW(kTag, "http header fetch failed path=%s elapsed_ms=%lld", path.c_str(),
                     (long long)(time_utils::NowMs() - started_ms));
            DropConnectionLocked();
            return false;
        }
        const int status = esp_http_client_get_status_code(client);

        body_out.clear();
        if (content_length > static_cast<int64_t>(kMaxResponseBytes)) {
            ESP_LOGW(kTag, "http response refused path=%s content_length=%lld limit=%u", path.c_str(),
                     (long long)content_length, (unsigned)kMaxResponseBytes);
            DropConnectionLocked();
            return false;
        }
        if (content_length > 0)
            body_out.reserve(static_cast<size_t>(content_length));
        bool failed = false;
        char buf[1024];
        while (true) {
            int n = esp_http_client_read(client, buf, sizeof(buf));
            if (n < 0) {
                ESP_LOGW(kTag, "http read failed path=%s elapsed_ms=%lld ret=%d", path.c_str(),
                         (long long)(time_utils::NowMs() - started_ms), n);
                failed = true;
                break;
            }
            if (n == 0)
                break;
            body_out.insert(body_out.end(), buf, buf + n);
            if (body_out.size() > kMaxResponseBytes) {
                ESP_LOGW(kTag, "http response exceeded path=%s limit=%u", path.c_str(), (unsigned)kMaxResponseBytes);
                failed = true;
                break;
            }
        }
        if (!failed && content_length > 0 && body_out.size() != static_cast<size_t>(content_length)) {
            ESP_LOGW(kTag, "http response short path=%s elapsed_ms=%lld bytes=%u expected=%lld", path.c_str(),
                     (long long)(time_utils::NowMs() - started_ms), (unsigned)body_out.size(),
                     (long long)content_length);
            failed = true;
        }
        if (failed) {
            DropConnectionLocked();
            return false;
        }
        // 成功:close 但不 cleanup,keep-alive 保留 socket 供后续请求复用。
        esp_http_client_close(client);

        if (status / 100 == 2) {
            consecutive_401_.store(0, std::memory_order_relaxed);
            return true;
        }
        LogErrorEnvelope(path, status, body_out);
        if (status == 401 && need_auth) {
            const int count = consecutive_401_.fetch_add(1, std::memory_order_relaxed) + 1;
            ESP_LOGW(kTag, "unauthorized path=%s count=%d threshold=%d", path.c_str(), count,
                     kUnauthorizedResetThreshold);
            UnauthorizedCb cb;
            if (count >= kUnauthorizedResetThreshold) {
                std::lock_guard<std::mutex> lock(state_mutex_);
                cb = unauthorized_cb_;
            }
            if (cb) {
                cb();
            }
            return false;
        }
        consecutive_401_.store(0, std::memory_order_relaxed);
        if (status == 426)
            ESP_LOGE(kTag, "protocol rejected path=%s protocol=%d action=upgrade_firmware", path.c_str(), kProtocol);
        return false;
    }
    return false;
}

bool ApiClient::PostJson(const std::string& path, const std::string& body, bool need_auth,
                         const std::function<bool(cJSON*)>& parse) {
    if (body.empty()) {
        ESP_LOGW(kTag, "request alloc failed path=%s", path.c_str());
        return false;
    }
    std::vector<uint8_t> resp;
    if (!DoRequest(path, HTTP_METHOD_POST, body, resp, need_auth, kSyncTimeoutMs))
        return false;
    cJSON* root = cJSON_ParseWithLength(reinterpret_cast<const char*>(resp.data()), resp.size());
    if (!root) {
        ESP_LOGW(kTag, "response invalid path=%s reason=json_parse bytes=%u", path.c_str(), (unsigned)resp.size());
        return false;
    }
    cJSON*     data = cJSON_GetObjectItemCaseSensitive(root, proto::kData);
    const bool ok   = JsonBool(root, proto::kSuccess, false) && cJSON_IsObject(data) && parse(data);
    if (!ok)
        ESP_LOGW(kTag, "response invalid path=%s reason=envelope_or_data", path.c_str());
    cJSON_Delete(root);
    return ok;
}

// ─── 设备协议端点 ────────────────────────────────────────────────
bool ApiClient::Register(RegisterResult& out) {
    std::string mac;
    {
        std::lock_guard<std::mutex> lock(state_mutex_);
        mac = mac_;
    }
    cJSON* root = NewProtocolBody();
    if (root)
        cJSON_AddStringToObject(root, proto::kMac, mac.c_str());
    return PostJson("/api/device/register", PrintBody(root), /*need_auth=*/false, [&](cJSON* data) {
        out                = {};
        out.id             = JsonString(data, proto::kDeviceId);
        out.device_secret  = JsonString(data, proto::kSecret);
        out.pair_code      = JsonString(data, proto::kPairCode);
        out.server_time_ms = JsonInt64(data, proto::kServerTime);
        return !out.id.empty() && !out.device_secret.empty() && !out.pair_code.empty();
    });
}

bool ApiClient::Sync(const std::string& manifest_version, const Telemetry& tel, DeviceState& out) {
    cJSON* root = NewProtocolBody();
    if (root) {
        cJSON_AddStringToObject(root, proto::kManifestVersion, manifest_version.c_str());
        cJSON* t = cJSON_AddObjectToObject(root, proto::kTelemetry);
        if (!t) {
            cJSON_Delete(root);
            root = nullptr;
        } else {
            if (tel.battery_pct >= 0)
                cJSON_AddNumberToObject(t, proto::kBatteryPct, tel.battery_pct);
            if (tel.rssi_dbm != 0)
                cJSON_AddNumberToObject(t, proto::kRssiDbm, tel.rssi_dbm);
            if (!tel.fw_version.empty())
                cJSON_AddStringToObject(t, proto::kFwVersion, tel.fw_version.c_str());
            if (!tel.wake_reason.empty())
                cJSON_AddStringToObject(t, proto::kWakeReason, tel.wake_reason.c_str());
            if (!tel.current_content_id.empty())
                cJSON_AddStringToObject(t, proto::kCurrentContentId, tel.current_content_id.c_str());
        }
    }
    return PostJson("/api/device/sync", PrintBody(root), /*need_auth=*/true,
                    [&](cJSON* data) { return ParseDeviceState(data, out); });
}

bool ApiClient::ChangeGroup(GroupAction action, const std::string& group_id, const std::string& manifest_version,
                            DeviceState& out) {
    const char* name = action == GroupAction::kNext ? "next" : action == GroupAction::kPrev ? "prev" : "select";
    if (action == GroupAction::kSelect && group_id.empty())
        return false;
    cJSON* root = NewProtocolBody();
    if (root) {
        cJSON_AddStringToObject(root, proto::kAction, name);
        if (action == GroupAction::kSelect)
            cJSON_AddStringToObject(root, proto::kGroupId, group_id.c_str());
        cJSON_AddStringToObject(root, proto::kManifestVersion, manifest_version.c_str());
    }
    return PostJson("/api/device/group", PrintBody(root), /*need_auth=*/true,
                    [&](cJSON* data) { return ParseDeviceState(data, out); });
}

bool ApiClient::DownloadBlob(const Blob& blob, std::vector<uint8_t>& out) {
    if (!IsBlobHash(blob.hash))
        return false;
    const std::string path = "/api/device/blobs/" + blob.hash;
    if (!DoRequest(path, HTTP_METHOD_GET, "", out, /*need_auth=*/true, kBlobTimeoutMs))
        return false;
    if (out.size() != static_cast<size_t>(blob.size)) {
        ESP_LOGW(kTag, "blob rejected reason=size bytes=%u expected=%d", (unsigned)out.size(), blob.size);
        return false;
    }
    uint8_t digest[32];
    if (mbedtls_sha256(out.data(), out.size(), digest, 0) != 0)
        return false;
    char hex[65];
    for (int i = 0; i < 32; ++i)
        std::snprintf(hex + i * 2, 3, "%02x", digest[i]);
    if (blob.hash != hex) {
        ESP_LOGW(kTag, "blob rejected reason=sha256 hash=%.12s", blob.hash.c_str());
        return false;
    }
    return true;
}

ApiClient& DefaultClient() {
    static ApiClient client;
    return client;
}

void Init(const std::string& server_url, const std::string& mac, const std::string& device_secret) {
    DefaultClient().Configure(server_url, mac, device_secret);
}

void SetServerUrl(const std::string& url) {
    DefaultClient().SetServerUrl(url);
}

void SetSecret(const std::string& secret) {
    DefaultClient().SetSecret(secret);
}

void SetUnauthorizedHandler(UnauthorizedCb cb) {
    DefaultClient().SetUnauthorizedHandler(std::move(cb));
}

void ResetConnection() {
    DefaultClient().ResetConnection();
}

bool Register(RegisterResult& out) {
    return DefaultClient().Register(out);
}

bool Sync(const std::string& manifest_version, const Telemetry& tel, DeviceState& out) {
    return DefaultClient().Sync(manifest_version, tel, out);
}

bool ChangeGroup(GroupAction action, const std::string& group_id, const std::string& manifest_version,
                 DeviceState& out) {
    return DefaultClient().ChangeGroup(action, group_id, manifest_version, out);
}

bool DownloadBlob(const Blob& blob, std::vector<uint8_t>& out) {
    return DefaultClient().DownloadBlob(blob, out);
}

}  // namespace api
