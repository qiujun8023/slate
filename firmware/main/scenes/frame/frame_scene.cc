#include "scenes/frame/frame_scene.h"

#include <esp_log.h>
#include <cstdio>
#include <vector>

#include "drivers/audio/audio_player.h"
#include "drivers/display/epd_ssd1683.h"
#include "events/event_bus.h"
#include "events/ui_event_log.h"
#include "scenes/core/scene_stack.h"
#include "scenes/settings/settings_scene.h"
#include "scenes/splash/splash_scene.h"
#include "storage/cache/cache.h"
#include "ui/frame_view.h"
#include "ui/status_bar.h"
#include "ui/theme.h"
#include "utils/utf8_utils.h"

namespace {
constexpr char kTag[] = "frame";

std::string ShortGroupName(const char* raw) {
    constexpr size_t kMaxChars = 8;
    if (!raw || raw[0] == '\0')
        return "内容组";
    const std::string s(raw);
    const std::string prefix = util::Utf8PrefixChars(s, kMaxChars);
    if (prefix.size() >= s.size())
        return s;
    return prefix + "…";
}

std::string MarkedGroupName(const char* raw) {
    return "《" + ShortGroupName(raw) + "》";
}

std::string FormatGroupSyncCaption(const UiEvent& e) {
    char buf[96];
    switch (e.u.group_sync.mode) {
        case GroupSyncStatusMode::kCycleTarget:
            return "切到" + MarkedGroupName(e.u.group_sync.name);
        case GroupSyncStatusMode::kCycleCacheHit:
            return "已切到" + MarkedGroupName(e.u.group_sync.name);
        case GroupSyncStatusMode::kCycleDownloading:
            if (e.u.group_sync.total > 0) {
                std::snprintf(buf, sizeof(buf), "下载%s %u/%u", MarkedGroupName(e.u.group_sync.name).c_str(),
                              e.u.group_sync.current, e.u.group_sync.total);
                return buf;
            }
            return "下载" + MarkedGroupName(e.u.group_sync.name);
        case GroupSyncStatusMode::kCurrentGroupUpdating:
            if (e.u.group_sync.total > 0) {
                std::snprintf(buf, sizeof(buf), "更新当前组 %u/%u", e.u.group_sync.current, e.u.group_sync.total);
                return buf;
            }
            return "更新当前组";
        case GroupSyncStatusMode::kInitialGroupDownloading:
            if (e.u.group_sync.total > 0) {
                std::snprintf(buf, sizeof(buf), "下载%s %u/%u", MarkedGroupName(e.u.group_sync.name).c_str(),
                              e.u.group_sync.current, e.u.group_sync.total);
                return buf;
            }
            return "下载内容组";
        case GroupSyncStatusMode::kTargetGroupSaving:
            if (e.u.group_sync.total > 0) {
                std::snprintf(buf, sizeof(buf), "应用%s %u/%u", MarkedGroupName(e.u.group_sync.name).c_str(),
                              e.u.group_sync.current, e.u.group_sync.total);
                return buf;
            }
            return "应用" + MarkedGroupName(e.u.group_sync.name);
        case GroupSyncStatusMode::kCurrentGroupSaving:
            if (e.u.group_sync.total > 0) {
                std::snprintf(buf, sizeof(buf), "应用当前组 %u/%u", e.u.group_sync.current, e.u.group_sync.total);
                return buf;
            }
            return "应用当前组";
        case GroupSyncStatusMode::kCycleFailed:
            return "切换失败，保留当前组";
    }
    return "";
}
}  // namespace

FrameScene::FrameScene(SceneContext& ctx, const char* gid, int content_count)
    : gid_(gid ? gid : ""), content_count_(content_count) {
    idx_ = ctx.current_frame_seq ? ctx.current_frame_seq() : 0;
    if (content_count_ <= 0)
        content_count_ = 0;
    if (content_count_ > 0 && (idx_ < 0 || idx_ >= content_count_))
        idx_ = 0;
    ESP_LOGD(kTag, "construct gid=%s count=%d idx=%d", gid_.c_str(), content_count_, idx_);
}

FrameScene::~FrameScene() = default;

void FrameScene::OnEnter(SceneContext& ctx) {
    ESP_LOGD(kTag, "enter gid=%s count=%d idx=%d first_loaded=%d", gid_.c_str(), content_count_, idx_,
             first_loaded_ ? 1 : 0);
    if (!ctx.epd->Lock(2000)) {
        ESP_LOGW(kTag, "enter failed reason=epd_lock_timeout");
        return;
    }

    root_ = CreateFullscreenRoot();

    frame_view_ = std::make_unique<FrameView>(root_);

    empty_label_ = lv_label_create(root_);
    lv_obj_set_style_text_font(empty_label_, &Zfull_16, 0);
    lv_obj_set_style_text_color(empty_label_, lv_color_black(), 0);
    lv_obj_set_style_text_align(empty_label_, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_style_text_line_space(empty_label_, 8, 0);
    lv_label_set_long_mode(empty_label_, LV_LABEL_LONG_WRAP);
    lv_obj_set_width(empty_label_, LV_HOR_RES - 32);
    lv_label_set_text(empty_label_, "内容组暂无内容\n\n请在管理端添加内容");
    lv_obj_align(empty_label_, LV_ALIGN_CENTER, 0, 0);
    lv_obj_add_flag(empty_label_, LV_OBJ_FLAG_HIDDEN);

    status_bar_ = std::make_unique<StatusBar>(root_);
    RefreshStatusBarFromSensors(ctx, *status_bar_);
    ApplyEmptyState();

    ctx.epd->Unlock();

    if (!gid_.empty() && content_count_ > 0) {
        LoadFrame(ctx, idx_, /*force_full*/ first_load_full_refresh_, AudioBehavior::RestartIfAvailable);
    } else {
        ESP_LOGI(kTag, "content empty gid_empty=%d count=%d", gid_.empty() ? 1 : 0, content_count_);
        if (ctx.audio)
            ctx.audio->Stop();
        SyncRender(ctx, /*force_full*/ first_load_full_refresh_);
    }
}

void FrameScene::OnExit(SceneContext& ctx) {
    ESP_LOGD(kTag, "exit gid=%s idx=%d", gid_.c_str(), idx_);
    if (ctx.audio)
        ctx.audio->Stop();
    DestroyRoot(ctx, root_, [this]() {
        frame_view_.reset();
        status_bar_.reset();
        empty_label_ = nullptr;
    });
}

void FrameScene::OnEvent(SceneContext& ctx, const UiEvent& e) {
    if (evt::log::DebugEnabled(kTag)) {
        char detail[128];
        evt::log::Describe(e, detail, sizeof(detail));
        ESP_LOGD(kTag, "event kind=%s detail=%s root=%p gid=%s idx=%d count=%d", evt::log::KindName(e.kind), detail,
                 root_, gid_.c_str(), idx_, content_count_);
    }
    if (!root_) {
        ESP_LOGW(kTag, "event ignored reason=no_root kind=%s", evt::log::KindName(e.kind));
        return;
    }
    switch (e.kind) {
        case UiEventKind::kButtonShort: {
            switch (e.u.button.btn) {
                case ButtonId::kUp:
                    ESP_LOGD(kTag, "button short btn=up action=prev_frame");
                    PrevFrame(ctx);
                    break;
                case ButtonId::kDown:
                case ButtonId::kEnter:
                    ESP_LOGD(kTag, "button short btn=%s action=next_frame", evt::log::ButtonName(e.u.button.btn));
                    NextFrame(ctx);
                    break;
            }
            break;
        }
        case UiEventKind::kButtonLong: {
            switch (e.u.button.btn) {
                case ButtonId::kUp:
                    ESP_LOGD(kTag, "button long btn=up action=prev_group");
                    CycleGroup(ctx, /*next=*/false);
                    break;
                case ButtonId::kDown:
                    ESP_LOGD(kTag, "button long btn=down action=next_group");
                    CycleGroup(ctx, /*next=*/true);
                    break;
                case ButtonId::kEnter:
                    ESP_LOGD(kTag, "button long btn=enter action=settings");
                    ctx.stack->RequestPush(std::make_unique<SettingsScene>());
                    break;
            }
            break;
        }
        case UiEventKind::kChargeChanged: {
            RefreshStatusBarForCharge(ctx, e);
            break;
        }
        case UiEventKind::kBatteryUpdated: {
            RefreshStatusBarForBattery(ctx, e);
            break;
        }
        case UiEventKind::kWifiStateChanged: {
            RefreshStatusBarForWifi(ctx, e);
            break;
        }
        case UiEventKind::kSyncProgress: {
            // FrameScene 使用带内容组名称的 kGroupSyncStatus；旧进度事件保留给启动页兜底。
            break;
        }
        case UiEventKind::kGroupSyncStatus: {
            HandleGroupSyncStatus(ctx, e);
            break;
        }
        case UiEventKind::kSyncFinished: {
            RestoreStatusBarCaption(ctx);
            break;
        }
        case UiEventKind::kSyncedGroupReady: {
            if (e.u.group.gid[0] == '\0')
                break;
            const bool same_group      = (gid_ == e.u.group.gid);
            const bool content_changed = e.u.group.content_changed;
            if (same_group && !content_changed) {
                content_count_ = e.u.group.content_count > 0 ? e.u.group.content_count : 0;
                if (content_count_ > 0 && (idx_ < 0 || idx_ >= content_count_))
                    idx_ = 0;
                break;
            }
            if (!same_group) {
                RebindGroup(ctx, e.u.group.gid, e.u.group.content_count);
            } else {
                content_count_ = e.u.group.content_count > 0 ? e.u.group.content_count : 0;
                if (content_count_ > 0 && (idx_ < 0 || idx_ >= content_count_))
                    idx_ = 0;
            }
            if (content_count_ > 0) {
                SyncRender(ctx, [this]() { ApplyEmptyState(); }, /*force_full*/ false);
                LoadFrame(ctx, idx_, /*force_full*/ true,
                          same_group ? AudioBehavior::StopIfUnavailable : AudioBehavior::RestartIfAvailable);
            } else {
                if (ctx.audio)
                    ctx.audio->Stop();
                SyncRender(ctx, [this]() { ApplyEmptyState(); }, /*force_full*/ true);
            }
            break;
        }
        case UiEventKind::kMinuteTick:
            RefreshStatusBarAndRender(ctx, status_bar_.get());
            break;
        case UiEventKind::kUnbound: {
            ESP_LOGW(kTag, "device unbound action=splash");
            ctx.stack->RequestReplace(std::make_unique<SplashScene>());
            break;
        }
        default:
            break;
    }
}

void FrameScene::NextFrame(SceneContext& ctx) {
    if (content_count_ <= 0) {
        ESP_LOGD(kTag, "next frame ignored reason=empty count=%d", content_count_);
        return;
    }
    const int old = idx_;
    idx_          = (idx_ + 1) % content_count_;
    ESP_LOGD(kTag, "next frame from=%d to=%d count=%d", old, idx_, content_count_);
    LoadFrame(ctx, idx_, /*force_full*/ false, AudioBehavior::RestartIfAvailable);
}

void FrameScene::PrevFrame(SceneContext& ctx) {
    if (content_count_ <= 0) {
        ESP_LOGD(kTag, "prev frame ignored reason=empty count=%d", content_count_);
        return;
    }
    const int old = idx_;
    idx_          = (idx_ - 1 + content_count_) % content_count_;
    ESP_LOGD(kTag, "prev frame from=%d to=%d count=%d", old, idx_, content_count_);
    LoadFrame(ctx, idx_, /*force_full*/ false, AudioBehavior::RestartIfAvailable);
}

void FrameScene::CycleGroup(SceneContext& ctx, bool next) {
    ESP_LOGI(kTag, "cycle group direction=%s gid=%s idx=%d", next ? "next" : "prev", gid_.c_str(), idx_);
    SyncRenderIfChanged(
        ctx, [this]() { return status_bar_ && status_bar_->SetCaption("切换内容组…"); },
        /*force_full*/ false);
    if (ctx.cycle_group)
        ctx.cycle_group(next);
}

void FrameScene::HandleGroupSyncStatus(SceneContext& ctx, const UiEvent& e) {
    ESP_LOGD(kTag, "group sync status mode=%s gid=%s name=%s current=%u total=%u",
             evt::log::GroupSyncStatusModeName(e.u.group_sync.mode), e.u.group_sync.gid, e.u.group_sync.name,
             static_cast<unsigned>(e.u.group_sync.current), static_cast<unsigned>(e.u.group_sync.total));
    const std::string caption = FormatGroupSyncCaption(e);
    if (caption.empty())
        return;
    SyncRenderIfChanged(
        ctx, [this, &caption]() { return status_bar_ && status_bar_->SetCaption(caption); },
        /*force_full*/ false);
}

void FrameScene::RefreshStatusBarForCharge(SceneContext& ctx, const UiEvent& e) {
    const auto& c   = e.u.charge;
    int         pct = -1;
    if (!c.no_battery && ctx.read_battery) {
        int mv = 0;
        ctx.read_battery(&mv, &pct);
    }
    SyncRenderIfChanged(
        ctx, [this, pct, &c]() { return status_bar_ && status_bar_->SetBattery(pct, c.charging, c.full); },
        /*force_full*/ false);
}

void FrameScene::RefreshStatusBarForBattery(SceneContext& ctx, const UiEvent& e) {
    bool charging = false;
    bool full     = false;
    if (ctx.read_charge) {
        const auto snap = ctx.read_charge();
        charging        = snap.charging;
        full            = snap.full;
    }
    SyncRenderIfChanged(
        ctx,
        [this, &e, charging, full]() {
            return status_bar_ && status_bar_->SetBattery(e.u.battery.pct, charging, full);
        },
        /*force_full*/ false);
}

void FrameScene::RefreshStatusBarForWifi(SceneContext& ctx, const UiEvent& e) {
    SyncRenderIfChanged(
        ctx, [this, &e]() { return status_bar_ && status_bar_->SetWifi(e.u.wifi.connected, e.u.wifi.rssi); },
        /*force_full*/ false);
}

void FrameScene::RestoreStatusBarCaption(SceneContext& ctx) {
    if (cached_status_bar_text_.empty())
        return;
    SyncRenderIfChanged(
        ctx, [this]() { return status_bar_ && status_bar_->SetCaption(cached_status_bar_text_); },
        /*force_full*/ false);
}

void FrameScene::RebindGroup(SceneContext& ctx, const char* gid, int content_count) {
    (void)ctx;
    ESP_LOGI(kTag, "rebind group old_gid=%s new_gid=%s old_count=%d new_count=%d", gid_.c_str(), gid ? gid : "",
             content_count_, content_count);
    gid_           = gid ? gid : "";
    content_count_ = content_count > 0 ? content_count : 0;
    idx_           = 0;
    if (ctx.clear_current_frame)
        ctx.clear_current_frame();
    first_loaded_ = false;
}

void FrameScene::LoadFrame(SceneContext& ctx, int idx, bool force_full, AudioBehavior audio_behavior) {
    ESP_LOGD(kTag, "load frame begin gid=%s idx=%d count=%d force_full=%d audio=%s", gid_.c_str(), idx, content_count_,
             force_full ? 1 : 0,
             audio_behavior == AudioBehavior::RestartIfAvailable ? "restart_if_available" : "stop_if_unavailable");
    if (gid_.empty() || idx < 0 || content_count_ <= 0 || idx >= content_count_) {
        ESP_LOGW(kTag, "load frame failed reason=invalid_state gid_empty=%d idx=%d count=%d", gid_.empty() ? 1 : 0, idx,
                 content_count_);
        return;
    }

    std::vector<uint8_t> raw;
    if (!cache::ReadFrameImage(gid_, idx, raw) || raw.size() != static_cast<size_t>(FrameView::kRawBytes)) {
        ESP_LOGW(kTag, "load frame failed idx=%d reason=image_miss bytes=%u", idx, static_cast<unsigned>(raw.size()));
        if (ctx.audio)
            ctx.audio->Stop();
        return;
    }
    cache::FrameMeta meta;
    cache::ReadFrameMeta(gid_, idx, meta);

    if (!ctx.epd->Lock(2000)) {
        ESP_LOGW(kTag, "load frame failed idx=%d reason=epd_lock_timeout", idx);
        return;
    }
    ESP_LOGD(kTag, "lvgl refresh begin scene=frame idx=%d", idx);
    if (status_bar_)
        status_bar_->SetCaption(meta.status_bar_text);
    cached_status_bar_text_ = meta.status_bar_text;
    lv_refr_now(NULL);
    ESP_LOGD(kTag, "lvgl refresh done scene=frame idx=%d", idx);
    ctx.epd->Unlock();

    if (frame_view_)
        frame_view_->SetFrame(ctx.epd, raw);

    const bool full = force_full || (!first_loaded_ && first_load_full_refresh_);
    first_loaded_   = true;
    if (full)
        ctx.epd->RequestUrgentFullRefresh();
    else
        ctx.epd->RequestUrgentPartialRefresh();
    ESP_LOGD(kTag, "load frame refresh idx=%d full=%d first_loaded=%d", idx, full ? 1 : 0, first_loaded_ ? 1 : 0);

    if (ctx.audio) {
        std::vector<uint8_t> pcm;
        if (!meta.audio_hash.empty() && cache::ReadFrameAudio(gid_, idx, pcm) && !pcm.empty()) {
            if (audio_behavior == AudioBehavior::RestartIfAvailable) {
                ESP_LOGD(kTag, "audio play idx=%d bytes=%u", idx, static_cast<unsigned>(pcm.size()));
                ctx.audio->Play(pcm.data(), pcm.size());
            }
        } else {
            ESP_LOGD(kTag, "audio stop idx=%d has_audio=%d", idx, meta.audio_hash.empty() ? 0 : 1);
            ctx.audio->Stop();
        }
    }

    if (ctx.set_current_frame_from_meta)
        ctx.set_current_frame_from_meta(idx, meta);
}

void FrameScene::ApplyEmptyState() {
    const bool empty = (content_count_ <= 0);
    ESP_LOGD(kTag, "empty state empty=%d count=%d", empty ? 1 : 0, content_count_);
    if (frame_view_) {
        if (empty)
            frame_view_->Hide();
        else
            frame_view_->Show();
    }
    if (empty_label_) {
        if (empty)
            lv_obj_clear_flag(empty_label_, LV_OBJ_FLAG_HIDDEN);
        else
            lv_obj_add_flag(empty_label_, LV_OBJ_FLAG_HIDDEN);
    }
    if (empty && status_bar_) {
        status_bar_->SetCaption("");
        cached_status_bar_text_.clear();
    }
}
