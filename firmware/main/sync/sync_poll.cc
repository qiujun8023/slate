#include "sync/sync_service.h"

#include <esp_log.h>
#include <sdkconfig.h>

#include "bsp/board.h"
#include "events/event_bus.h"
#include "network/sntp.h"
#include "network/wifi.h"
#include "power/power_state.h"
#include "sync/sync_internal.h"
#include "utils/time_utils.h"

using sync_internal::kTag;

namespace {

api::Telemetry BuildTelemetry(const std::string& wake_reason) {
    api::Telemetry tel;
    uint16_t       mv  = 0;
    uint8_t        pct = 0;
    if (Board::Get().ReadBattery(&mv, &pct)) {
        tel.battery_pct = pct;
        evt::PostBatteryUpdated(mv, pct, evt::kNoWait);
    }
    tel.rssi_dbm    = Wifi::Get().GetRssi();
    tel.fw_version  = CONFIG_APP_PROJECT_VER;
    tel.wake_reason = wake_reason;
    if (power_state::CurrentFrameNeedsTimerWake()) {
        std::string      gid;
        std::string      manifest_version;
        cache::FrameMeta meta;
        if (cache::ReadStateMeta(gid, manifest_version) && !gid.empty() &&
            cache::ReadFrameMeta(gid, power_state::GetCurrentFrameSeq(), meta)) {
            tel.current_content_id = meta.content_id;
        }
    }
    return tel;
}

// 上报给服务端的 manifestVersion:只有本地确实缓存了该版本才发,否则发空串要完整 manifest。
std::string KnownManifestVersion() {
    std::string         gid;
    std::string         version;
    cache::ManifestMeta meta;
    if (!cache::ReadStateMeta(gid, version) || gid.empty() || version.empty())
        return "";
    if (!cache::ReadManifestMeta(gid, meta) || meta.manifest_version != version)
        return "";
    return version;
}

}  // namespace

bool SyncService::ClearSelectedGroup() {
    ClearCurrentGroup();
    power_state::ClearCurrentFrame();
    return cache::WriteStateMeta("", "");
}

bool SyncService::ApplyState(const api::DeviceState& state, SyncReason reason, bool& group_changed) {
    group_changed = false;
    if (!state.has_group)
        return ClearSelectedGroup();
    return SyncManifestAndFrames(state, reason, group_changed);
}

void SyncService::SyncOnce(SyncMode mode) {
    const int64_t started_ms = time_utils::NowMs();
    ESP_LOGI(kTag, "sync start mode=%s", SyncModeName(mode));
    if (mode == SyncMode::kBackgroundRefresh)
        power_state::RestoreCurrentFrameScheduleFromCache();

    evt::PostSyncStarted(evt::kNoWait);

    const api::Telemetry tel = BuildTelemetry(wake_reason_);
    if (ShouldStop())
        return;

    api::DeviceState state;
    if (!api::Sync(KnownManifestVersion(), tel, state)) {
        if (ShouldStop())
            return;
        ESP_LOGW(kTag, "sync request failed reason=offline_or_server");
        ESP_LOGI(kTag, "sync done mode=%s ok=0 group_changed=0 elapsed_ms=%lld", SyncModeName(mode),
                 (long long)(time_utils::NowMs() - started_ms));
        evt::PostSyncFinished(false, false, evt::kNoWait);
        return;
    }
    if (ShouldStop())
        return;
    sntp::ApplyServerTime(state.server_time_ms);

    const BoundState prev_bound = was_bound_.load();
    const BoundState next_bound = state.bound ? BoundState::kBound : BoundState::kUnbound;
    if (next_bound != prev_bound) {
        if (state.bound) {
            evt::PostSimple(UiEventKind::kBound, evt::kNoWait);
        } else {
            ESP_LOGW(kTag, "device unbound");
            evt::PostUnbound(state.pair_code, evt::kNoWait);
        }
        was_bound_.store(next_bound);
        if (state.bound) {
            unbound_since_ms_.store(0);
        } else {
            unbound_since_ms_.store(time_utils::NowMs());
        }
    }

    if (!state.bound) {
        evt::PostBootStage(BootStage::kAwaitingPair, nullptr, state.pair_code.c_str(), evt::kNoWait);
    } else if (!state.has_group) {
        evt::PostBootStage(BootStage::kAwaitingGroup, nullptr, nullptr, evt::kNoWait);
    }

    const SyncReason reason =
        mode == SyncMode::kBackgroundRefresh ? SyncReason::kBackgroundRefresh : SyncReason::kUserActive;
    bool       group_changed = false;
    const bool sync_ok       = ApplyState(state, reason, group_changed);

    ESP_LOGI(kTag, "sync done mode=%s ok=%d group_changed=%d elapsed_ms=%lld", SyncModeName(mode), sync_ok ? 1 : 0,
             group_changed ? 1 : 0, (long long)(time_utils::NowMs() - started_ms));
    evt::PostSyncFinished(sync_ok, group_changed, evt::kNoWait);
}

void SyncService::DoCycle(api::GroupAction action) {
    const char*   direction  = action == api::GroupAction::kPrev ? "prev" : "next";
    const int64_t started_ms = time_utils::NowMs();
    ESP_LOGI(kTag, "cycle start direction=%s", direction);
    evt::PostSyncStarted(evt::kNoWait);

    api::DeviceState state;
    if (ShouldStop())
        return;
    if (!api::ChangeGroup(action, "", KnownManifestVersion(), state)) {
        if (ShouldStop())
            return;
        ESP_LOGW(kTag, "cycle group failed direction=%s reason=offline_or_server", direction);
        ESP_LOGI(kTag, "cycle done direction=%s ok=0 group_changed=0 elapsed_ms=%lld", direction,
                 (long long)(time_utils::NowMs() - started_ms));
        evt::PostSyncFinished(false, false, evt::kNoWait);
        evt::PostGroupSyncStatus(GroupSyncStatusMode::kCycleFailed, "", "");
        return;
    }
    if (ShouldStop())
        return;
    sntp::ApplyServerTime(state.server_time_ms);

    if (state.has_group)
        evt::PostGroupSyncStatus(GroupSyncStatusMode::kCycleTarget, state.group_id, state.group_name);
    bool       group_changed = false;
    const bool sync_ok       = ApplyState(state, SyncReason::kCycle, group_changed);

    evt::PostSyncFinished(sync_ok, group_changed, evt::kNoWait);
    if (!sync_ok)
        evt::PostGroupSyncStatus(GroupSyncStatusMode::kCycleFailed, state.group_id, state.group_name);
    ESP_LOGI(kTag, "cycle done direction=%s ok=%d group_changed=%d elapsed_ms=%lld", direction, sync_ok ? 1 : 0,
             group_changed ? 1 : 0, (long long)(time_utils::NowMs() - started_ms));
}
