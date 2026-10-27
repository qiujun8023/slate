#include "power/power_state.h"

#include <esp_attr.h>
#include <esp_log.h>
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>

#include <cstring>
#include <ctime>

#include "drivers/display/framebuffer_ops.h"
#include "network/sntp.h"
#include "utils/scoped_mutex_lock.h"

namespace power_state {
namespace {

constexpr char kTag[] = "power_state";

// 最小 wake 间隔：太短会把 deep sleep 的省电优势磨没。
constexpr uint32_t kMinWakeIntervalSec = 60u;

// RTC slow memory 持久化变量。深睡跨越保留；cold boot 由 Init(true) 显式清零。
RTC_DATA_ATTR int64_t s_frame_refresh_at  = 0;
RTC_DATA_ATTR int     s_current_frame_seq = 0;

// 连续 timer wake 未能联系上服务器(WiFi/后端不可达,或 sync 失败)的次数。
// 用于指数退避下次 RTC timer 间隔,避免网络长期不可用时每 60s 空醒耗电。
// 成功同步清零;cold boot 清零;深睡跨越保留(退避需跨唤醒累计)。
RTC_DATA_ATTR uint32_t s_timer_wake_fail_count = 0;

RTC_DATA_ATTR bool s_cache_commit_interrupted = false;

// 退避位移上限:60s << 6 ≈ 64min,与下方 kMaxBackoffWakeSec 共同封顶。
constexpr uint32_t kMaxBackoffShift   = 6;
constexpr uint64_t kMaxBackoffWakeSec = 3600;

constexpr uint32_t     kStatusBarSnapshotMagic                             = 0x53544231u;  // "STB1"
RTC_DATA_ATTR uint32_t s_status_bar_magic                                  = 0;
RTC_DATA_ATTR uint32_t s_status_bar_hash                                   = 0;
RTC_DATA_ATTR uint8_t  s_status_bar_snapshot[epd::kStatusBarSnapshotBytes] = {};

SemaphoreHandle_t StateMutex() {
    static StaticSemaphore_t s_mutex_buf;
    static SemaphoreHandle_t s_mutex = xSemaphoreCreateMutexStatic(&s_mutex_buf);
    return s_mutex;
}

uint32_t HashBytes(const uint8_t* data, size_t len) {
    uint32_t h = 2166136261u;
    for (size_t i = 0; i < len; ++i) {
        h ^= data[i];
        h *= 16777619u;
    }
    return h;
}

}  // namespace

void Init(bool cold_boot) {
    if (!cold_boot)
        return;
    ScopedMutexLock lock(StateMutex());
    s_frame_refresh_at         = 0;
    s_current_frame_seq        = 0;
    s_timer_wake_fail_count    = 0;
    s_cache_commit_interrupted = false;
    s_status_bar_magic         = 0;
    s_status_bar_hash          = 0;
    std::memset(s_status_bar_snapshot, 0, sizeof(s_status_bar_snapshot));
}

CurrentFrameSchedule GetCurrentFrameSchedule() {
    CurrentFrameSchedule schedule;
    ScopedMutexLock      lock(StateMutex());
    schedule.refresh_at = s_frame_refresh_at;
    return schedule;
}

void SetCurrentFrameSchedule(const CurrentFrameSchedule& schedule) {
    ScopedMutexLock lock(StateMutex());
    s_frame_refresh_at = schedule.refresh_at > 0 ? schedule.refresh_at : 0;
}

int GetCurrentFrameSeq() {
    ScopedMutexLock lock(StateMutex());
    const int       seq = s_current_frame_seq;
    return seq < 0 ? 0 : seq;
}

void SetCurrentFrameSeq(int seq) {
    ScopedMutexLock lock(StateMutex());
    s_current_frame_seq = seq < 0 ? 0 : seq;
}

void SetCurrentFrameFromMeta(int seq, const cache::FrameMeta& meta) {
    SetCurrentFrameSchedule({.refresh_at = meta.refresh_at});
    SetCurrentFrameSeq(seq);
    // flash 去重：current_frame_seq 仅在与已持久化值不同时才写。后台定时刷新每次唤醒
    // 都展示同一帧，旧实现每次都写 LittleFS，长期磨损 flash；读取不伤寿命，先读再判。
    int persisted = 0;
    if (cache::ReadCurrentFrameSeq(persisted) && persisted == seq)
        return;
    cache::WriteCurrentFrameSeq(seq);
}

bool CacheCommitInterrupted() {
    ScopedMutexLock lock(StateMutex());
    return s_cache_commit_interrupted;
}

void SetCacheCommitInterrupted(bool interrupted) {
    ScopedMutexLock lock(StateMutex());
    s_cache_commit_interrupted = interrupted;
}

void ClearCurrentFrame() {
    SetCurrentFrameSchedule({});
    SetCurrentFrameSeq(0);
    cache::WriteCurrentFrameSeq(0);
}

bool RestoreCurrentFrameScheduleFromCache() {
    std::string gid;
    std::string manifest_version;
    if (!cache::ReadStateMeta(gid, manifest_version) || gid.empty()) {
        ClearCurrentFrame();
        return false;
    }

    int content_count = 0;
    if (!cache::ReadManifestContentCount(gid, content_count) || content_count <= 0) {
        ClearCurrentFrame();
        return false;
    }

    int seq = 0;
    cache::ReadCurrentFrameSeq(seq);
    if (seq < 0 || seq >= content_count)
        seq = 0;

    cache::FrameMeta meta;
    if (!cache::ReadFrameMeta(gid, seq, meta)) {
        SetCurrentFrameSchedule({});
        SetCurrentFrameSeq(seq);
        return false;
    }

    SetCurrentFrameFromMeta(seq, meta);
    return true;
}

bool CurrentFrameNeedsTimerWake() {
    ScopedMutexLock lock(StateMutex());
    return s_frame_refresh_at > 0;
}

uint32_t ComputeNextWakeSec() {
    ScopedMutexLock lock(StateMutex());
    if (s_frame_refresh_at <= 0)
        return 0;
    const uint32_t shift = s_timer_wake_fail_count > kMaxBackoffShift ? kMaxBackoffShift : s_timer_wake_fail_count;
    // 不可达退避:连续 timer wake 失败时指数抬高下限,封顶 kMaxBackoffWakeSec。
    const uint64_t backoff = static_cast<uint64_t>(kMinWakeIntervalSec) << shift;
    const uint64_t floor   = backoff > kMaxBackoffWakeSec ? kMaxBackoffWakeSec : backoff;
    // 未对时则无法换算绝对时刻,按下限唤醒,醒来后由 serverTime 校准。
    const int64_t delta = sntp::TimeSynced() ? s_frame_refresh_at - static_cast<int64_t>(time(nullptr)) : 0;
    if (delta <= static_cast<int64_t>(floor))
        return static_cast<uint32_t>(floor);
    return delta > static_cast<int64_t>(UINT32_MAX) ? UINT32_MAX : static_cast<uint32_t>(delta);
}

void RecordTimerWakeResult(bool success) {
    ScopedMutexLock lock(StateMutex());
    if (success) {
        s_timer_wake_fail_count = 0;
    } else if (s_timer_wake_fail_count < kMaxBackoffShift) {
        ++s_timer_wake_fail_count;
    }
}

bool SaveStatusBarSnapshot(const uint8_t* data, size_t len) {
    if (!data || len != epd::kStatusBarSnapshotBytes)
        return false;
    const uint32_t  hash = HashBytes(data, len);
    ScopedMutexLock lock(StateMutex());
    // magic 是提交标记:先清无效,写完 snapshot/hash 后再恢复,Load 只接受完整快照。
    s_status_bar_magic = 0;
    std::memcpy(s_status_bar_snapshot, data, epd::kStatusBarSnapshotBytes);
    s_status_bar_hash  = hash;
    s_status_bar_magic = kStatusBarSnapshotMagic;
    ESP_LOGD(kTag, "saved status bar snapshot hash=%08lx", static_cast<unsigned long>(hash));
    return true;
}

bool LoadStatusBarSnapshot(uint8_t* out, size_t len) {
    if (!out || len != epd::kStatusBarSnapshotBytes)
        return false;
    uint32_t magic = 0;
    uint32_t hash  = 0;
    {
        ScopedMutexLock lock(StateMutex());
        magic = s_status_bar_magic;
        hash  = s_status_bar_hash;
        if (magic == kStatusBarSnapshotMagic) {
            std::memcpy(out, s_status_bar_snapshot, epd::kStatusBarSnapshotBytes);
        }
    }
    if (magic != kStatusBarSnapshotMagic)
        return false;
    return HashBytes(out, len) == hash;
}

void ClearStatusBarSnapshot() {
    ScopedMutexLock lock(StateMutex());
    s_status_bar_magic = 0;
    s_status_bar_hash  = 0;
}

}  // namespace power_state
