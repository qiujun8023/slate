#pragma once

#include <cstddef>
#include <cstdint>

namespace sync_internal {

inline constexpr char    kTag[]                = "sync";
inline constexpr int     kBoundPollSec         = 60;
inline constexpr int     kStopWaitMs           = 30000;
inline constexpr int     kUnboundFastPollSec   = 10;
inline constexpr int     kUnboundMediumPollSec = 30;
inline constexpr int     kUnboundSlowPollSec   = 60;
inline constexpr int64_t kUnboundFastMs        = 10LL * 60 * 1000;
inline constexpr int64_t kUnboundMediumMs      = 30LL * 60 * 1000;
inline constexpr size_t  kCacheMinFreeBytes    = 1024 * 1024;
inline constexpr int     kMaxCachedGroups      = 4;

uint8_t ClampProgressCount(int value);

}  // namespace sync_internal
