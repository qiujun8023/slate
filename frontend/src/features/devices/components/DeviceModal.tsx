// 设备详情 modal — 取代了原 /devices/:did 单独路由页。
//
// 内容:
//   ① 标题区:在线状态点 + MAC + 设备名(铅笔图标 inline 改名)
//   ② 在播:当前在播组 + 切换 selector
//   ③ metadata 网格:电量 / 信号 / 固件 / 心跳
//   ④ 危险区:解绑(把 owner 置 null,素材保留)
//
// /devices/:did URL 仍可用 — 由 Dashboard 监听 useParams 自动打开 modal。

import { useCallback } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import type { Device } from '@/api';
import { usePatchDevice } from '@/features/devices/query/device-queries';
import { useGroups } from '@/features/groups/query/group-queries';
import { useToast } from '@/components/feedback/toast-context';
import { useInlineRename } from '@/hooks/useInlineRename';
import { useTimeAgo } from '@/hooks/useTimeAgo';
import { useDeviceOnline } from '@/features/devices/hooks/useDeviceOnline';
import { useUnbindDeviceWithConfirm } from '@/features/devices/hooks/useUnbindDeviceWithConfirm';
import { dialogContentWideCls, dialogOverlayCls } from '@/components/ui/styles/dialog';
import { getApiErrorMessage } from '@/lib/api-errors';
import { DeviceDangerZone } from './DeviceDangerZone';
import { DeviceGroupSelector } from './DeviceGroupSelector';
import { DeviceNameHeader } from './DeviceNameHeader';
import { DeviceStatusGrid } from './DeviceStatusGrid';

interface DeviceModalProps {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  device: Device;
}

export function DeviceModal({ open, onOpenChange, device }: DeviceModalProps) {
  const groups = useGroups();
  const patch = usePatchDevice(device.id);
  const toast = useToast();
  const onUnbindSuccess = useCallback(() => onOpenChange(false), [onOpenChange]);
  const { unbindWithConfirm, isPending: unbindPending } = useUnbindDeviceWithConfirm(
    device,
    onUnbindSuccess
  );

  const {
    editing: editingName,
    draft: draftName,
    setDraft: setDraftName,
    startEditing,
    commit,
    handleKeyDown,
  } = useInlineRename(device.name ?? '', async (name) => {
    try {
      await patch.mutateAsync({ name });
      toast.success('已改名');
    } catch (err) {
      toast.error('改名失败', getApiErrorMessage(err));
      throw err;
    }
  });

  const online = useDeviceOnline(device.lastSeenAt);
  const lastSeenAgo = useTimeAgo(device.lastSeenAt);

  function changeGroup(value: string) {
    if (patch.isPending) return;
    const next = value === '__none__' ? null : value;
    if (next === device.groupId) return;
    patch.mutate(
      { groupId: next },
      {
        onSuccess: () => toast.success(next ? '已切换在播' : '已清空在播'),
        onError: (err) => toast.error('切换失败', getApiErrorMessage(err)),
      }
    );
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className={dialogOverlayCls} />
        <Dialog.Content className={dialogContentWideCls}>
          <DeviceNameHeader
            device={device}
            online={online}
            editingName={editingName}
            draftName={draftName}
            onDraftNameChange={setDraftName}
            onStartEditing={startEditing}
            onCommit={commit}
            onKeyDown={handleKeyDown}
            pending={patch.isPending}
          />

          <div className="flex-1 overflow-y-auto px-6 sm:px-7 py-5 space-y-6">
            <DeviceGroupSelector
              groups={groups.data ?? []}
              value={device.groupId}
              onChange={changeGroup}
              disabled={patch.isPending}
            />
            <DeviceStatusGrid device={device} online={online} lastSeenAgo={lastSeenAgo} />
            <DeviceDangerZone pending={unbindPending} onUnbind={unbindWithConfirm} />
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
