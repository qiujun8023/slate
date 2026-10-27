#include "sync/sync_service.h"

#include <esp_log.h>
#include <freertos/FreeRTOS.h>
#include <freertos/event_groups.h>
#include <freertos/task.h>

#include <utility>

#include "events/event_bus.h"
#include "sync/sync_internal.h"
#include "utils/time_utils.h"

namespace {
constexpr int BIT_TRIGGER      = BIT0;
constexpr int BIT_STOP         = BIT1;
constexpr int BIT_CYCLE_NEXT   = BIT2;
constexpr int BIT_CYCLE_PREV   = BIT3;
constexpr int BIT_WAKE_REFRESH = BIT4;
}  // namespace

SyncService& SyncService::Get() {
    static SyncService s;
    return s;
}

void SyncService::TaskEntry(void* arg) {
    auto* self = static_cast<SyncService*>(arg);
    self->Loop();
    {
        std::lock_guard<std::mutex> lock(self->task_mutex_);
        if (self->task_handle_ == xTaskGetCurrentTaskHandle())
            self->task_handle_ = nullptr;
    }
    if (self->exit_sem_)
        xSemaphoreGive(self->exit_sem_);
    vTaskDelete(nullptr);
}

void SyncService::Start(std::string wake_reason, InitialSync initial_sync) {
    if (running_.load(std::memory_order_acquire))
        return;
    {
        std::lock_guard<std::mutex> lock(task_mutex_);
        if (task_handle_) {
            ESP_LOGW(sync_internal::kTag, "start ignored reason=previous_task_running");
            return;
        }
    }
    wake_reason_ = std::move(wake_reason);
    std::atomic_thread_fence(std::memory_order_release);
    if (!event_group_) {
        event_group_ = xEventGroupCreate();
        if (!event_group_) {
            ESP_LOGE(sync_internal::kTag, "event group create failed");
            return;
        }
    }
    if (!exit_sem_) {
        exit_sem_ = xSemaphoreCreateBinary();
        if (!exit_sem_) {
            ESP_LOGE(sync_internal::kTag, "exit semaphore create failed");
            return;
        }
    }
    xSemaphoreTake(exit_sem_, 0);
    running_.store(true, std::memory_order_release);
    {
        std::lock_guard<std::mutex> lock(task_mutex_);
        BaseType_t ok = xTaskCreatePinnedToCore(&TaskEntry, "slate_sync", 10 * 1024, this, 4, &task_handle_, 0);
        if (ok != pdPASS) {
            running_.store(false, std::memory_order_release);
            task_handle_ = nullptr;
            ESP_LOGE(sync_internal::kTag, "task create failed name=slate_sync");
            return;
        }
    }
    switch (initial_sync) {
        case InitialSync::kNone:
            break;
        case InitialSync::kUserActive:
            Trigger(SyncMode::kUserActive);
            break;
        case InitialSync::kBackgroundRefresh:
            Trigger(SyncMode::kBackgroundRefresh);
            break;
    }
}

void SyncService::Stop() {
    const bool was_running = running_.exchange(false, std::memory_order_acq_rel);
    bool       has_task    = false;
    {
        std::lock_guard<std::mutex> lock(task_mutex_);
        has_task = task_handle_ != nullptr;
    }
    if (!was_running && !has_task)
        return;
    if (was_running && event_group_) {
        xEventGroupSetBits(event_group_, BIT_STOP);
    }
    if (exit_sem_ && xSemaphoreTake(exit_sem_, pdMS_TO_TICKS(sync_internal::kStopWaitMs)) != pdTRUE) {
        ESP_LOGW(sync_internal::kTag, "stop timeout elapsed_ms=%d", sync_internal::kStopWaitMs);
        return;
    }
    {
        std::lock_guard<std::mutex> lock(task_mutex_);
        task_handle_ = nullptr;
    }
}

void SyncService::Trigger(SyncMode mode) {
    const int bit = mode == SyncMode::kBackgroundRefresh ? BIT_WAKE_REFRESH : BIT_TRIGGER;
    if (event_group_)
        xEventGroupSetBits(event_group_, bit);
}

const char* SyncService::SyncModeName(SyncMode mode) {
    switch (mode) {
        case SyncMode::kUserActive:
            return "user_active";
        case SyncMode::kBackgroundRefresh:
            return "background_refresh";
    }
    return "unknown";
}

void SyncService::RequestUserActiveSync() {
    Trigger(SyncMode::kUserActive);
}

void SyncService::CycleNext() {
    if (event_group_)
        xEventGroupSetBits(event_group_, BIT_CYCLE_NEXT);
}

void SyncService::CyclePrev() {
    if (event_group_)
        xEventGroupSetBits(event_group_, BIT_CYCLE_PREV);
}

std::string SyncService::CurrentGroupId() const {
    return CurrentGroupSnapshot();
}

std::string SyncService::CurrentGroupSnapshot() const {
    std::lock_guard<std::mutex> lock(current_group_mutex_);
    std::string                 gid = current_group_;
    return gid;
}

void SyncService::SetCurrentGroup(const std::string& gid) {
    std::lock_guard<std::mutex> lock(current_group_mutex_);
    current_group_ = gid;
}

void SyncService::ClearCurrentGroup() {
    std::lock_guard<std::mutex> lock(current_group_mutex_);
    current_group_.clear();
}

int SyncService::NextIntervalSec() const {
    if (was_bound_.load() == BoundState::kBound)
        return sync_internal::kBoundPollSec;
    const int64_t elapsed = time_utils::NowMs() - unbound_since_ms_.load();
    if (elapsed < sync_internal::kUnboundFastMs)
        return sync_internal::kUnboundFastPollSec;
    if (elapsed < sync_internal::kUnboundMediumMs)
        return sync_internal::kUnboundMediumPollSec;
    return sync_internal::kUnboundSlowPollSec;
}

bool SyncService::ShouldStop() const {
    return !running_.load(std::memory_order_acquire);
}

void SyncService::PostSyncedGroupReady(const std::string& gid, const std::string& name, int content_count,
                                       bool content_changed) {
    evt::PostGroupReady(UiEventKind::kSyncedGroupReady, gid, name, content_count, content_changed);
}

void SyncService::Loop() {
    while (running_.load(std::memory_order_acquire)) {
        const int         interval_s = NextIntervalSec();
        const EventBits_t bits       = xEventGroupWaitBits(
            event_group_, BIT_TRIGGER | BIT_STOP | BIT_CYCLE_NEXT | BIT_CYCLE_PREV | BIT_WAKE_REFRESH, pdTRUE, pdFALSE,
            pdMS_TO_TICKS(interval_s * 1000));
        if (bits & BIT_STOP)
            break;
        if (!running_.load(std::memory_order_acquire))
            break;

        // 标记突发进行中：SleepManager 据此阻止 idle 睡眠打断正在进行的下载。
        in_flight_.store(true, std::memory_order_release);
        if (bits & BIT_CYCLE_NEXT) {
            DoCycle(api::GroupAction::kNext);
        } else if (bits & BIT_CYCLE_PREV) {
            DoCycle(api::GroupAction::kPrev);
        } else if (bits & BIT_WAKE_REFRESH) {
            SyncOnce(SyncMode::kBackgroundRefresh);
        } else {
            SyncOnce(SyncMode::kUserActive);
        }
        in_flight_.store(false, std::memory_order_release);

        // 一次 sync 突发(sync/group → 各帧 blob)结束,释放持久连接回收 mbedTLS 内存。
        // 连接只在单次突发内复用,突发之间(轮询间隔以分钟计)不保活,避免常驻占内存。
        api::ResetConnection();
    }
}
