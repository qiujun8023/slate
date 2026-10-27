#include "scenes/bg_refresh/bg_refresh_scene.h"

#include <esp_log.h>
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>

#include <array>
#include <atomic>
#include <cstring>
#include <memory>
#include <new>
#include <vector>

#include "drivers/display/epd_ssd1683.h"
#include "drivers/display/framebuffer_ops.h"
#include "events/event_bus.h"
#include "power/power_state.h"
#include "storage/cache/cache.h"
#include "ui/frame_view.h"
#include "ui/status_bar.h"
#include "ui/theme.h"

namespace {
constexpr char kTag[] = "bg_refresh";
constexpr int  kBpr   = FrameView::kWidth / 8;

// 后台刷新整体硬截止：从进场到完成的总时长上限。WiFi 连接 + poll + 拉帧 + EPD 刷新
// 都算在内。超时直接投 kBgRefreshDone 回睡，不再依赖 10min idle Tick 兜底，封住
// 「sync 卡住 → 持续亮屏连网耗电」的窗口。
constexpr int kBgRefreshDeadlineMs = 40000;

void PostBgRefreshDone() {
    evt::PostSimple(UiEventKind::kBgRefreshDone);
}

void PostBgRefreshDoneOnce(const std::shared_ptr<std::atomic<bool>>& done_posted) {
    if (done_posted && done_posted->exchange(true, std::memory_order_acq_rel))
        return;
    PostBgRefreshDone();
}

struct WatcherContext {
    EpdSsd1683*                        epd = nullptr;
    std::shared_ptr<std::atomic<bool>> done_posted;
};

void WatcherEntry(void* arg) {
    std::unique_ptr<WatcherContext> ctx(static_cast<WatcherContext*>(arg));
    auto*                           epd        = ctx ? ctx->epd : nullptr;
    constexpr int                   kTimeoutMs = 8000;
    if (epd)
        epd->WaitForRefreshIdle(kTimeoutMs);
    auto done_posted = ctx ? ctx->done_posted : std::shared_ptr<std::atomic<bool>>();
    PostBgRefreshDoneOnce(done_posted);
    vTaskDelete(nullptr);
}

void UpdateFrameSchedule(int seq, const cache::FrameMeta& meta) {
    power_state::SetCurrentFrameFromMeta(seq, meta);
}

// 截止看护任务：等到 kBgRefreshDeadlineMs；其间一旦 done_posted 置位(正常 finish)就提前退出，
// 否则到点强制 PostBgRefreshDoneOnce。复用 WatcherContext(epd 置空,只用 done_posted)。
// 自删除 + unique_ptr 释放 ctx,与 WatcherEntry 同模式,无泄漏。
void DeadlineEntry(void* arg) {
    std::unique_ptr<WatcherContext> ctx(static_cast<WatcherContext*>(arg));
    auto                            done_posted = ctx ? ctx->done_posted : std::shared_ptr<std::atomic<bool>>();
    int                             waited      = 0;
    const auto finished = [&] { return done_posted && done_posted->load(std::memory_order_acquire); };
    while (waited < kBgRefreshDeadlineMs && !finished()) {
        vTaskDelay(pdMS_TO_TICKS(200));
        waited += 200;
    }
    if (!finished()) {
        ESP_LOGW(kTag, "deadline reached elapsed_ms=%d action=force_done", kBgRefreshDeadlineMs);
        // 不在此处 RecordTimerWakeResult：与 OnEvent 的上报存在时序竞态(渲染跨过截止时
        // 会先 true 再 false 重复计数)。失败退避由「连不上服务器」(app.cc net_ok=false)与
        // OnEvent 的 kSyncFinished(ok) 覆盖；「连上但每次卡满截止」是罕见失败模式，
        // 仅靠 40s 截止回睡兜底、不计入退避（已知次要限制）。
        PostBgRefreshDoneOnce(done_posted);
    }
    vTaskDelete(nullptr);
}

}  // namespace

BgRefreshScene::~BgRefreshScene() = default;

void BgRefreshScene::OnEnter(SceneContext& ctx) {
    done_posted_->store(false, std::memory_order_release);
    previous_screen_seeded_ = SeedPreviousFrame(ctx);
    state_                  = State::kWaiting;
    StartDeadlineWatchdog();
}

void BgRefreshScene::StartDeadlineWatchdog() {
    auto* ctx = new (std::nothrow) WatcherContext{nullptr, done_posted_};
    if (!ctx) {
        ESP_LOGW(kTag, "deadline watchdog alloc failed");
        return;  // 退化到 SleepManager 的 idle/看门狗兜底
    }
    BaseType_t ok = xTaskCreatePinnedToCore(&DeadlineEntry, "bg_refresh_deadline", 2048, ctx, 2, nullptr, 0);
    if (ok != pdPASS) {
        delete ctx;
        ESP_LOGW(kTag, "deadline watchdog create failed");
    }
}

void BgRefreshScene::OnExit(SceneContext& ctx) {
    DestroyRoot(ctx, root_, [this] { status_bar_.reset(); });
}

void BgRefreshScene::OnEvent(SceneContext& ctx, const UiEvent& e) {
    if (e.kind != UiEventKind::kSyncFinished || state_ != State::kWaiting)
        return;

    // 上报本次 timer wake 的联网结果：ok 清零退避计数，失败递增。配合 app.cc 网络
    // 建立失败分支，让持续不可达的设备指数拉长唤醒间隔，而非每 ttl 空醒。
    power_state::RecordTimerWakeResult(e.u.sync.ok);

    if (!e.u.sync.ok || !e.u.sync.group_changed) {
        Finish();
        return;
    }

    if (!previous_screen_seeded_) {
        ESP_LOGW(kTag, "render skipped reason=previous_seed_incomplete");
        Finish();
        return;
    }

    state_ = State::kRendering;
    if (!RenderChangedFrame(ctx)) {
        Finish();
    }
}

bool BgRefreshScene::SeedPreviousFrame(SceneContext& ctx) {
    if (!ctx.epd)
        return false;

    std::array<uint8_t, epd::kStatusBarSnapshotBytes> status_bar{};
    const bool status_ok = power_state::LoadStatusBarSnapshot(status_bar.data(), status_bar.size());
    if (!status_ok) {
        ESP_LOGW(kTag, "seed skipped reason=status_snapshot_missing");
        return false;
    }

    std::string gid;
    std::string manifest_version;
    if (!cache::ReadStateMeta(gid, manifest_version) || gid.empty()) {
        ESP_LOGW(kTag, "seed skipped reason=cached_group_missing");
        return false;
    }

    int content_count = 0;
    if (!cache::ReadManifestContentCount(gid, content_count) || content_count <= 0) {
        ESP_LOGW(kTag, "seed skipped reason=manifest_missing");
        return false;
    }

    const int seq = power_state::GetCurrentFrameSeq();
    if (seq < 0 || seq >= content_count) {
        ESP_LOGW(kTag, "seed skipped reason=seq_out_of_range seq=%d count=%d", seq, content_count);
        return false;
    }

    std::vector<uint8_t> raw;
    if (!cache::ReadFrameImage(gid, seq, raw) || raw.size() != static_cast<size_t>(FrameView::kRawBytes)) {
        ESP_LOGW(kTag, "seed skipped reason=image_miss seq=%d bytes=%u", seq, static_cast<unsigned>(raw.size()));
        return false;
    }

    const int y = theme::kStatusBarHeight;
    const int h = FrameView::kHeight - y;
    ctx.epd->SeedPreviousRaw1bpp(0, 0, epd::kStatusBarSnapshotWidth, epd::kStatusBarSnapshotHeight, status_bar.data(),
                                 status_bar.size());
    ctx.epd->SeedPreviousRaw1bpp(0, y, FrameView::kWidth, h, raw.data() + y * kBpr, h * kBpr);
    return true;
}

bool BgRefreshScene::ResolveCurrentFrame(std::string& gid, int& seq, int& content_count) {
    std::string manifest_version;
    if (!cache::ReadStateMeta(gid, manifest_version) || gid.empty()) {
        ESP_LOGW(kTag, "render skipped reason=cached_group_missing");
        return false;
    }
    if (!cache::ReadManifestContentCount(gid, content_count) || content_count <= 0) {
        ESP_LOGW(kTag, "render skipped reason=manifest_missing");
        return false;
    }

    seq = power_state::GetCurrentFrameSeq();
    if (seq < 0 || seq >= content_count) {
        seq = 0;
    }
    return true;
}

bool BgRefreshScene::RenderChangedFrame(SceneContext& ctx) {
    if (!ctx.epd)
        return false;

    std::string gid;
    int         seq           = 0;
    int         content_count = 0;
    if (!ResolveCurrentFrame(gid, seq, content_count))
        return false;

    std::vector<uint8_t> raw;
    if (!cache::ReadFrameImage(gid, seq, raw) || raw.size() != static_cast<size_t>(FrameView::kRawBytes)) {
        ESP_LOGW(kTag, "render skipped reason=image_miss seq=%d bytes=%u", seq, static_cast<unsigned>(raw.size()));
        return false;
    }

    cache::FrameMeta meta;
    cache::ReadFrameMeta(gid, seq, meta);
    UpdateFrameSchedule(seq, meta);

    if (!ctx.epd->Lock(2000)) {
        ESP_LOGW(kTag, "render failed reason=epd_lock_timeout");
        return false;
    }

    root_ = CreateFullscreenRoot();
    lv_obj_set_height(root_, theme::kStatusBarHeight);

    status_bar_ = std::make_unique<StatusBar>(root_);
    status_bar_->SetCaption(meta.status_bar_text);
    RefreshStatusBarFromSensors(ctx, *status_bar_);
    lv_refr_now(NULL);

    // Background refresh uses LVGL only for the status bar. The frame body is
    // written as raw 1bpp data so it exactly matches the cached screen format.
    const int y = theme::kStatusBarHeight;
    const int h = FrameView::kHeight - y;
    ctx.epd->WriteRaw1bpp(0, y, FrameView::kWidth, h, raw.data() + y * kBpr, h * kBpr);
    ctx.epd->RequestUrgentPartialRefresh();
    ctx.epd->Unlock();

    StartWatcher(ctx.epd);
    return true;
}

void BgRefreshScene::StartWatcher(EpdSsd1683* epd) {
    auto* ctx = new (std::nothrow) WatcherContext{epd, done_posted_};
    if (!ctx) {
        ESP_LOGW(kTag, "watcher alloc failed action=finish");
        Finish();
        return;
    }
    BaseType_t ok = xTaskCreatePinnedToCore(&WatcherEntry, "bg_refresh_watch", 2048, ctx, 2, nullptr, 0);
    if (ok != pdPASS) {
        delete ctx;
        ESP_LOGW(kTag, "watcher create failed action=finish");
        Finish();
    }
}

void BgRefreshScene::Finish() {
    if (state_ == State::kDone)
        return;
    state_ = State::kDone;
    PostBgRefreshDoneOnce(done_posted_);
}
