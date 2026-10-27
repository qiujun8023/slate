#pragma once

// RTC slow memory 持久化的电源状态。深睡跨越保留，掉电后清零。
//
// 用途：
//   - 记录当前帧在服务端下次重新渲染的时刻(refresh_at)
//   - 避免每次 wake 都走完整 onboarding（cold boot 时数值为 0，按默认策略）
//
// 选 RTC slow RAM 而不是 NVS 的原因：写次数高（每次睡都更新）、不耐 flash 寿命。
//
// 当前帧调度状态机：
//   - FrameScene / BgRefreshScene 展示某帧后调用 SetCurrentFrameFromMeta(seq, meta)，
//     同时更新 RTC slow memory 与 LittleFS 中的 current_frame_seq。
//   - SleepManager 睡前调用 ComputeNextWakeSec()；只有 meta.refresh_at 非 0 的动态帧或屏幕
//     待重绘时才配置 RTC timer，静态帧只靠按键/插电唤醒后再同步。
//   - Timer wake 的后台刷新路径先 RestoreCurrentFrameScheduleFromCache()，再只上报/刷新当前帧。
//   - 组被清空或内容不可用时 ClearCurrentFrame() 把序号和动态调度都回到静态默认态。

#include <cstddef>
#include <cstdint>

#include "storage/cache/cache.h"

namespace power_state {

struct CurrentFrameSchedule {
    int64_t refresh_at = 0;  // Unix 秒；0 表示静态帧
};

// Explicitly reset RTC slow-memory state on cold boot. Deep-sleep wake keeps it.
void Init(bool cold_boot);

// 当前展示帧的刷新策略。FrameScene::LoadFrame 写入；后台同步拿到新 manifest 时
// SyncService 也会更新 refresh_at。静态帧不会自己变更,不配置定时唤醒,避免空耗电。
CurrentFrameSchedule GetCurrentFrameSchedule();
void                 SetCurrentFrameSchedule(const CurrentFrameSchedule& schedule);

int  GetCurrentFrameSeq();
void SetCurrentFrameSeq(int seq);
bool CurrentFrameNeedsTimerWake();
void SetCurrentFrameFromMeta(int seq, const cache::FrameMeta& meta);
void ClearCurrentFrame();

// 从 LittleFS cache 恢复当前帧序号和动态刷新策略。用于 deep sleep 前兜底，
// 防止 RTC slow memory 因 reset/cold boot 丢失后下一轮 timer wake 被关闭。
bool RestoreCurrentFrameScheduleFromCache();

// 当前动态帧的下次 RTC timer wakeup 间隔（秒）= refresh_at - now，至少 60s。
// 0 表示静态帧。连续 timer wake 联系不上服务器时按指数退避抬高下限，封顶 ~1h。
// 屏幕待重绘时另按补画退避唤醒(60s 起翻倍,最多 3 次),两者取较早者。
uint32_t ComputeNextWakeSec();

// 记录一次 timer wake 的同步结果。success=true 清零退避计数；false 递增（封顶）。
// 由后台刷新路径调用：网络建立失败 / sync 完成(ok 或失败) 时各上报一次。
void RecordTimerWakeResult(bool success);

// 屏幕待重绘：缓存提交前置位，确认物理屏幕已显示新缓存后清除（前台提交成功、后台刷新
// 等到 EPD 刷新完成）。置位期间缓存可能已换成新内容而屏幕仍是旧画面：下一次成功同步
// 必须按内容已变化处理，后台刷新不能用缓存当旧画面做局刷，须全刷。
// 清除时同时清零补画退避;RecordRedrawAttempt 在每次后台补画尝试时递增退避。
bool ScreenRedrawPending();
void SetScreenRedrawPending(bool pending);
void RecordRedrawAttempt();

// 睡前最后一次刷到物理屏上的状态栏 1bpp 快照。用于 timer wake 后重建
// prev_buffer_ 的 0~24 行，让后台 partial refresh 的 old/new 输入真实一致。
bool SaveStatusBarSnapshot(const uint8_t* data, size_t len);
bool LoadStatusBarSnapshot(uint8_t* out, size_t len);
void ClearStatusBarSnapshot();

}  // namespace power_state
