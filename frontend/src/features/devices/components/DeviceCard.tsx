// 设备卡片：可拖拽排序 + 在线状态点 + 电量/信号 + 底部操作行。

import { memo, useCallback } from 'react';
import { Wifi, Frame, Trash2 } from 'lucide-react';
import type { Device, Group } from '@/api';
import { DragHandle } from '@/components/ui/DragHandle';
import { IconBlock } from '@/components/ui/IconBlock';
import { rssiLabel } from '@/features/devices/lib/rssi';
import { useTimeAgo } from '@/hooks/useTimeAgo';
import { useDeviceOnline } from '@/features/devices/hooks/useDeviceOnline';
import { BatteryLevelIcon } from './BatteryLevelIcon';
import { useUnbindDeviceWithConfirm } from '@/features/devices/hooks/useUnbindDeviceWithConfirm';
import { cn } from '@/lib/cn';
import { useSortableStyle } from '@/components/dnd/useSortableStyle';

export const DeviceCard = memo(function DeviceCard({
  device,
  groups,
  onOpen,
}: {
  device: Device;
  groups: Group[] | undefined;
  onOpen: (deviceId: string) => void;
}) {
  const online = useDeviceOnline(device.lastSeenAt);
  const currentGroup = groups?.find((g) => g.id === device.groupId);
  const groupName = currentGroup?.name;
  const battery = device.batteryPct;
  const lowBattery = battery != null && battery < 20;

  const { unbindWithConfirm, isPending: unbindPending } = useUnbindDeviceWithConfirm(device);

  const { attributes, listeners, setNodeRef, style, isDragging } = useSortableStyle(device.id);

  const playingContents = currentGroup?.contentCount;

  const onUnbind = useCallback(
    async (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      await unbindWithConfirm();
    },
    [unbindWithConfirm]
  );

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'craft-card flex flex-col overflow-hidden',
        isDragging && 'shadow-drag opacity-90'
      )}
      data-hoverable="true"
    >
      <button
        onClick={() => onOpen(device.id)}
        className="block w-full text-left px-5 pt-5 pb-4 sm:px-6 sm:pt-6 sm:pb-4 hover:bg-cream transition-colors"
      >
        <div className="flex items-start gap-3">
          <IconBlock size="lg" tone="soft">
            <Frame size={24} />
          </IconBlock>
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-2">
              <h3 className="font-serif text-[18px] font-bold leading-tight truncate tracking-tight">
                {device.name ?? '未命名'}
              </h3>
              <span className="inline-flex items-center gap-1.5 text-[11px] text-stone flex-shrink-0">
                <span className={cn('dot', online ? 'dot-online' : 'dot-offline')} />
                {online ? '在线' : '离线'}
              </span>
            </div>

            <p className="font-mono text-[11px] text-stone-light mt-1 truncate">
              {device.id.slice(0, 12)}
            </p>

            <p className="mt-2 truncate">
              <span className="font-sans text-[9px] uppercase tracking-[0.18em] text-stone-light mr-1.5">
                在播
              </span>
              <span
                className={cn(
                  'font-serif text-[13px]',
                  groupName ? 'text-stone' : 'text-stone-light italic'
                )}
              >
                {groupName ?? '未选组'}
                {groupName && playingContents != null && ` · ${playingContents} 项`}
              </span>
            </p>

            <div className="mt-2 flex items-center gap-3.5 text-[12px] text-stone">
              {online ? (
                <>
                  <span className={cn('flex items-center gap-1.5', lowBattery && 'text-clay')}>
                    <BatteryLevelIcon level={battery} size={14} />
                    <span className="font-mono tabular-nums">
                      {battery != null ? `${battery}%` : '—'}
                    </span>
                  </span>
                  <span className="flex items-center gap-1.5">
                    <Wifi size={14} />
                    <span>{rssiLabel(device.rssiDbm)}</span>
                  </span>
                  <span className="text-stone-light text-[11px] ml-auto">刚刚</span>
                </>
              ) : (
                <LastSeenLabel lastSeenAt={device.lastSeenAt} />
              )}
            </div>
          </div>
        </div>
      </button>

      <div className="px-2 py-2 border-t border-line flex items-center min-h-[38px]">
        <DragHandle attributes={attributes} listeners={listeners} />

        <span className="flex-1" />

        <button
          type="button"
          onClick={onUnbind}
          disabled={unbindPending}
          aria-label="解绑"
          title="从账号解绑"
          className="p-1.5 text-stone hover:text-clay hover:bg-cream transition-colors disabled:opacity-50"
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  );
});

function LastSeenLabel({ lastSeenAt }: { lastSeenAt: string | null }) {
  const lastSeenAgo = useTimeAgo(lastSeenAt);
  return <span className="text-stone-light text-[11px]">上次心跳 {lastSeenAgo}</span>;
}
