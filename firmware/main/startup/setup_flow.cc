#include "startup/setup_flow.h"

#include <esp_log.h>
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>

#include "events/event_bus.h"
#include "network/sntp.h"
#include "network/wifi.h"
#include "power/shutdown.h"
#include "sync/api_client.h"
#include "utils/mac_utils.h"

namespace {
constexpr char kTag[]                  = "setup_flow";
constexpr int  kSaveSecretRetryCount   = 3;
constexpr int  kSaveSecretRetryDelayMs = 200;
}  // namespace

namespace setup_flow {

bool TryConnectAndSetup(cred::Credentials& c) {
    evt::PostBootStage(BootStage::kWifiConnecting, c.wifi_ssid.c_str());
    if (!Wifi::Get().Connect(c.wifi_ssid, c.wifi_pwd, 20000)) {
        ESP_LOGW(kTag, "wifi connect failed mode=sta");
        evt::PostBootStage(BootStage::kWifiFailed);
        return false;
    }

    evt::PostBootStage(BootStage::kSntp);
    sntp::Init();
    api::Init(c.server_url, util::WifiStaMacString(), c.device_secret);

    constexpr int kSntpWaitMs = 10000;
    int           waited      = 0;
    while (!sntp::TimeSynced() && waited < kSntpWaitMs) {
        vTaskDelay(pdMS_TO_TICKS(200));
        waited += 200;
    }
    if (!sntp::TimeSynced())
        ESP_LOGW(kTag, "sntp sync timeout elapsed_ms=%d impact=https_register_may_fail", kSntpWaitMs);

    if (c.device_secret.empty()) {
        evt::PostBootStage(BootStage::kRegistering);
        api::RegisterResult rr;
        if (!api::Register(rr)) {
            ESP_LOGW(kTag, "register failed reason=server_unreachable");
            evt::PostBootStage(BootStage::kServerUnreachable);
            return false;
        }
        sntp::ApplyServerTime(rr.server_time_ms);

        bool saved = false;
        for (int attempt = 1; attempt <= kSaveSecretRetryCount; ++attempt) {
            if (cred::SaveSecret(rr.id, rr.device_secret)) {
                saved = true;
                break;
            }
            ESP_LOGW(kTag, "save secret failed attempt=%d total=%d", attempt, kSaveSecretRetryCount);
            vTaskDelay(pdMS_TO_TICKS(kSaveSecretRetryDelayMs));
        }
        if (!saved) {
            ESP_LOGE(kTag, "save secret fatal action=restart");
            power_shutdown::GracefulRestart();
        }
        c.device_id     = rr.id;
        c.device_secret = rr.device_secret;
        api::SetSecret(rr.device_secret);
        evt::PostBootStage(BootStage::kAwaitingPair, nullptr, rr.pair_code.c_str());
    }
    return true;
}

}  // namespace setup_flow
