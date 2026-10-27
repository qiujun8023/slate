#include "sync/sync_internal.h"

namespace sync_internal {

uint8_t ClampProgressCount(int value) {
    if (value < 0)
        return 0;
    return static_cast<uint8_t>(value > 255 ? 255 : value);
}

}  // namespace sync_internal
