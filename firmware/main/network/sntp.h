#pragma once

// SNTP 对时:连上 STA 后立即调,后台同步时间。状态栏 %H:%M 用。

#include <cstdint>

namespace sntp {

void Init();                            // 启动 SNTP,设默认 timezone (Kconfig CST-8)
bool TimeSynced();                      // 是否已成功同步过(time(nullptr) > 2020 年视为成功)
void ApplyServerTime(int64_t unix_ms);  // SNTP 不可用或偏差明显时用服务端时间(Unix 毫秒)兜底

}  // namespace sntp
