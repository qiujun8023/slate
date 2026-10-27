import { useEffect, useState } from 'react';
import { isOnlineAt, onlineSnapshot } from '@/features/devices/lib/device-online';

const ONLINE_TIMEOUT_MARGIN_MS = 50;

export function useDeviceOnline(lastSeenAt: string | null): boolean {
  const [online, setOnline] = useState(() => isOnlineAt({ lastSeenAt: lastSeenAt }, Date.now()));

  useEffect(() => {
    const now = Date.now();
    const snapshot = onlineSnapshot(lastSeenAt, now);
    setOnline(snapshot.online);
    if (!snapshot.online || snapshot.offlineAt === null) return;

    const timeout = window.setTimeout(
      () => setOnline(false),
      Math.max(0, snapshot.offlineAt - Date.now() + ONLINE_TIMEOUT_MARGIN_MS)
    );
    return () => window.clearTimeout(timeout);
  }, [lastSeenAt]);

  return online;
}
