#include "network/sntp.h"

#include <esp_log.h>
#include <esp_netif_sntp.h>
#include <esp_sntp.h>
#include <sdkconfig.h>

#include <sys/time.h>
#include <cstdint>
#include <cstdlib>
#include <ctime>

namespace {
constexpr char kTag[] = "sntp";

constexpr time_t kSyncedAfterEpoch = 1577836800;  // 2020-01-01 UTC
constexpr long   kMaxClockDriftSec = 10;
}  // namespace

namespace sntp {

void Init() {
    esp_sntp_config_t cfg = ESP_NETIF_SNTP_DEFAULT_CONFIG("pool.ntp.org");
    cfg.start             = true;
    esp_netif_sntp_init(&cfg);

    setenv("TZ", CONFIG_SLATE_DEFAULT_TIMEZONE, 1);
    tzset();
}

bool TimeSynced() {
    time_t now = time(nullptr);
    return now > kSyncedAfterEpoch;
}

void ApplyServerTime(int64_t unix_ms) {
    const time_t server = static_cast<time_t>(unix_ms / 1000);
    if (server <= kSyncedAfterEpoch) {
        ESP_LOGW(kTag, "server time rejected value=%lld", (long long)unix_ms);
        return;
    }
    const time_t now  = time(nullptr);
    const long   diff = static_cast<long>(server - now);
    if (TimeSynced() && std::labs(diff) <= kMaxClockDriftSec)
        return;

    timeval tv{};
    tv.tv_sec  = server;
    tv.tv_usec = static_cast<suseconds_t>((unix_ms % 1000) * 1000);
    if (settimeofday(&tv, nullptr) != 0) {
        ESP_LOGW(kTag, "set time failed");
    }
}

}  // namespace sntp
