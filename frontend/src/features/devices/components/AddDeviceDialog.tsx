// 按设备屏上的 6 位配对码绑定设备。
//
// 流程：用户拿到一台已经联网但未绑定的 Slate 设备 → 设备屏上显示配对码 →
// 用户在此对话框输入 → 后端找到 device、把 owner 设为当前用户、轮换 pairCode。
//
// 命名留到绑定后在设备列表 PATCH name。

import { useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { ArrowRight, KeyRound } from 'lucide-react';
import { useClaimByPairCode } from '@/features/devices/query/device-queries';
import { useToast } from '@/components/feedback/toast-context';
import { isValidPairCode, normalizePairCode } from '@/features/devices/lib/pair-code';
import { getApiErrorMessage, getApiErrorStatus } from '@/lib/api-errors';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { Spinner } from '@/components/ui/Spinner';
import { DialogHeader } from '@/components/ui/DialogHeader';
import { dialogContentCls, dialogOverlayCls } from '@/components/ui/styles/dialog';

interface AddDeviceDialogProps {
  open: boolean;
  onOpenChange: (o: boolean) => void;
}

export function AddDeviceDialog({ open, onOpenChange }: AddDeviceDialogProps) {
  const [code, setCode] = useState('');
  const claim = useClaimByPairCode();
  const toast = useToast();

  const codeValid = isValidPairCode(code);

  function reset() {
    setCode('');
  }

  async function onSubmit(e: { preventDefault(): void }) {
    e.preventDefault();
    if (!codeValid) return;
    try {
      const device = await claim.mutateAsync({ pairCode: normalizePairCode(code) });
      // 后端 claim 时若 owner 已有相册会自动绑第一个，无相册则后续 create 会反向绑；
      // 这里只给出与实际后端行为一致的概要提示，不做额外引导（用户可在设备列表看进度）。
      toast.success(
        '设备已绑定',
        device.groupId ? '设备屏将开始同步相册' : '请创建一个相册，设备屏会自动同步'
      );
      reset();
      onOpenChange(false);
    } catch (err) {
      const status = getApiErrorStatus(err);
      if (status === 404) {
        toast.error('配对码无效', '请核对设备屏上的码，或在设备上长按 ENTER 工厂重置后重试。');
      } else if (status === 403) {
        toast.error('该设备已被其他账号绑定', '在设备上工厂重置后再试。');
      } else {
        toast.error('绑定失败', getApiErrorMessage(err));
      }
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) reset();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className={dialogOverlayCls} />
        <Dialog.Content className={dialogContentCls}>
          <DialogHeader
            icon={<KeyRound size={24} />}
            title="添加设备"
            description="在设备屏上查看 6 位配对码，输入此处即绑定。"
            className="mb-6"
          />

          <form onSubmit={onSubmit} className="space-y-5">
            <Input
              label="配对码"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="K7M9X2"
              autoFocus
              autoComplete="off"
              spellCheck={false}
              maxLength={8}
              hint={code && !codeValid ? undefined : '6 位字母+数字，可带短横线'}
              error={code && !codeValid ? '配对码格式不正确' : undefined}
              className="font-mono uppercase tracking-[0.2em] text-center"
            />

            <div className="flex items-center justify-end gap-3 pt-2">
              <Dialog.Close asChild>
                <Button variant="outline" type="button">
                  取消
                </Button>
              </Dialog.Close>
              <Button
                type="submit"
                disabled={!codeValid || claim.isPending}
                iconRight={claim.isPending ? undefined : <ArrowRight size={14} />}
              >
                {claim.isPending ? <Spinner /> : '绑定'}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
