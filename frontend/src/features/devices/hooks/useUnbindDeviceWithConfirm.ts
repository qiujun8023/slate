import { useCallback } from 'react';
import type { Device } from '@/api';
import { useConfirmAction } from '@/hooks/useMutationAction';
import { useUnbindDevice } from '@/features/devices/query/device-queries';

export function useUnbindDeviceWithConfirm(device: Device, onSuccess?: () => void) {
  const { mutate, isPending } = useUnbindDevice();

  const confirmUnbind = useConfirmAction<string>({
    isPending,
    getConfirmOptions: useCallback(
      () => ({
        title: '解绑这台设备？',
        description: `${device.name ?? device.mac} 将从你的账号移除。素材保留，设备屏会切回配对码状态。`,
        destructive: true,
        confirmText: '解绑',
      }),
      [device.mac, device.name]
    ),
    run: useCallback((deviceId, callbacks) => mutate(deviceId, callbacks), [mutate]),
    successToast: { message: '已解绑', hint: '设备屏会显示新配对码。' },
    errorToast: '解绑失败',
    onSuccess: useCallback(() => onSuccess?.(), [onSuccess]),
  });
  const unbindWithConfirm = useCallback(() => {
    void confirmUnbind(device.id);
  }, [confirmUnbind, device.id]);

  return { unbindWithConfirm, isPending };
}
