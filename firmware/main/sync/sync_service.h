#pragma once

// 后台同步：POST /api/device/sync(或 /group)拿到 DeviceState,manifest 版本变化时按 hash 增量拉帧。
// 状态/进度通过 EventBus 反馈：
//   - SyncStarted       每轮开始
//   - SyncFinished{ok}  每轮结束(含 304 noop)
//   - kSyncedGroupReady{gid,content_count}  当 selected_group 内容就绪

#include <atomic>
#include <mutex>
#include <string>
#include <vector>

#include <freertos/FreeRTOS.h>
#include <freertos/event_groups.h>
#include <freertos/semphr.h>
#include <freertos/task.h>

#include "storage/cache/cache.h"
#include "sync/api_client.h"

class SyncService {
   public:
    static SyncService& Get();

    enum class InitialSync {
        kNone,
        kUserActive,
        kBackgroundRefresh,
    };

    void Start(std::string wake_reason, InitialSync initial_sync = InitialSync::kNone);
    void Stop();

    // 主动触发一次前台 sync。用于后台刷新被充电/解绑宽限打断后转入 active 模式。
    void RequestUserActiveSync();

    // 设备主动 cycle 切组(scene 按键 callback 调)。
    // 内部置 BIT_CYCLE_NEXT/PREV,Loop 在唤醒时调 api::ChangeGroup 并同步返回的 manifest。
    void CycleNext();
    void CyclePrev();

    // 当前已就绪的 group_id(Scene::OnEnter 时读)
    std::string CurrentGroupId() const;

    // 是否正在执行一次 sync 突发(sync/group → 拉帧)。SleepManager 用它阻止
    // idle deep sleep 在大文件下载中途打断同步。只在单轮 SyncOnce/DoCycle 期间为真,
    // 突发之间(轮询间隔)为假,因此不会让设备永不睡(卡死由看门狗兜底)。
    bool IsBusy() const {
        return in_flight_.load(std::memory_order_acquire);
    }

   private:
    SyncService() = default;
    static void TaskEntry(void* arg);
    void        Loop();
    int         NextIntervalSec() const;
    enum class SyncMode { kUserActive, kBackgroundRefresh };
    enum class SyncReason { kUserActive, kBackgroundRefresh, kCycle };
    void               SyncOnce(SyncMode mode);
    void               Trigger(SyncMode mode);
    void               DoCycle(api::GroupAction action);
    static const char* SyncModeName(SyncMode mode);
    bool               ApplyState(const api::DeviceState& state, SyncReason reason, bool& group_changed);
    bool               ShouldStop() const;
    bool               SyncManifestAndFrames(const api::DeviceState& state, SyncReason reason, bool& group_changed);
    bool               ClearSelectedGroup();
    bool HandleCachedManifestHit(const std::string& gid, const std::string& manifest_version,
                                 const std::string& status_name, int content_count, const std::string& previous_current,
                                 const std::string& selected_group_id, SyncReason reason, bool content_changed);
    bool DownloadFramesToStage(cache::CacheWriter& writer, const std::string& gid, const api::Manifest& manifest,
                               const std::string& status_name, const std::string& previous_current,
                               const std::string& selected_group_id, SyncReason reason, int& total_updates);
    bool CommitStagedFrames(cache::CacheWriter& writer, const std::string& gid, const api::Manifest& manifest,
                            const std::string& group_name, const std::string& selected_group_id,
                            bool current_group_update, SyncReason reason, int total_updates, int old_content_count);
    void PostSyncedGroupReady(const std::string& gid, const std::string& name, int content_count, bool content_changed);
    std::string CurrentGroupSnapshot() const;
    void        SetCurrentGroup(const std::string& gid);
    void        ClearCurrentGroup();

    std::atomic<bool>    running_{false};
    std::atomic<bool>    in_flight_{false};
    mutable std::mutex   task_mutex_;
    mutable std::mutex   current_group_mutex_;
    EventGroupHandle_t   event_group_ = nullptr;
    SemaphoreHandle_t    exit_sem_    = nullptr;
    TaskHandle_t         task_handle_ = nullptr;
    mutable std::string  current_group_;
    std::string          wake_reason_;
    std::vector<uint8_t> download_buf_;
    enum class BoundState : uint8_t { kUnknown, kBound, kUnbound };
    // 跟踪 bound 翻转。Unknown 初始态保证首轮 unbound 也会 emit kUnbound。
    std::atomic<BoundState> was_bound_{BoundState::kUnknown};
    // 进入 unbound 状态的时刻,用于阶梯退避轮询间隔。
    std::atomic<int64_t> unbound_since_ms_{0};
};
