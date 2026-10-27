import { Wifi } from 'lucide-react';
import type { Device } from '@/api';
import { rssiLabel } from '@/features/devices/lib/rssi';
import { BatteryLevelIcon } from './BatteryLevelIcon';
import { DeviceMetaCard } from './DeviceMetaCard';

export function DeviceStatusGrid({
  device,
  online,
  lastSeenAgo,
}: {
  device: Device;
  online: boolean;
  lastSeenAgo: string;
}) {
  const battery = device.batteryPct;

  return (
    <section>
      <h3 className="font-sans text-[12px] uppercase tracking-wide text-stone mb-2">状态</h3>
      <div className="grid grid-cols-2 gap-3">
        <DeviceMetaCard
          icon={<BatteryLevelIcon level={battery} size={16} />}
          label="电量"
          value={online && battery != null ? `${battery}%` : '—'}
          warn={online && battery != null && battery < 20}
          stale={!online}
        />
        <DeviceMetaCard
          icon={<Wifi size={16} />}
          label="信号"
          value={online && device.rssiDbm != null ? `${device.rssiDbm} dBm` : '—'}
          hint={online ? rssiLabel(device.rssiDbm) : undefined}
          stale={!online}
        />
        <DeviceMetaCard label="固件" value={device.fwVersion ?? '—'} mono />
        <DeviceMetaCard label="心跳" value={lastSeenAgo} />
      </div>
    </section>
  );
}
