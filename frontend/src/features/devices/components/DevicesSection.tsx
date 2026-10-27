import { useCallback, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Cpu, MonitorSmartphone } from 'lucide-react';
import type { Device, Group } from '@/api';
import { Section } from '@/components/layout/Section';
import { useToast } from '@/components/feedback/toast-context';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { SortableGrid } from '@/components/dnd/SortableGrid';
import { Spinner } from '@/components/ui/Spinner';
import { AddDeviceDialog } from '@/features/devices/components/AddDeviceDialog';
import { DeviceCard } from '@/features/devices/components/DeviceCard';
import { DeviceModal } from '@/features/devices/components/DeviceModal';
import { useReorderDevices } from '@/features/devices/query/device-queries';
import { useDndOrder } from '@/components/dnd/useDndOrder';
import { getApiErrorMessage } from '@/lib/api-errors';
import { appRoutes } from '@/app/routes';

interface DevicesSectionProps {
  devices: Device[] | undefined;
  groups: Group[] | undefined;
  isPending: boolean;
}

export function DevicesSection({ devices, groups, isPending }: DevicesSectionProps) {
  const navigate = useNavigate();
  const { did } = useParams();
  const [addOpen, setAddOpen] = useState(false);
  const openDevice = devices?.find((device) => device.id === did);

  const openDeviceById = useCallback(
    (deviceId: string) => {
      navigate(appRoutes.device(deviceId));
    },
    [navigate]
  );

  const closeDeviceModal = useCallback(() => {
    navigate(appRoutes.home, { replace: true });
  }, [navigate]);

  const renderDevice = useCallback(
    (device: Device) => <DeviceCard device={device} groups={groups} onOpen={openDeviceById} />,
    [groups, openDeviceById]
  );

  const reorderDevices = useReorderDevices();
  const toast = useToast();
  const dnd = useDndOrder(
    devices,
    useCallback((device) => device.id, []),
    (newOrder, { commit, rollback }) =>
      reorderDevices.mutate(
        { order: newOrder },
        {
          onSuccess: commit,
          onError: (err) => {
            rollback();
            toast.error('排序保存失败', getApiErrorMessage(err));
          },
        }
      )
  );

  return (
    <>
      <Section
        title="设备"
        badge={<MonitorSmartphone size={18} />}
        subtitle="设备通过 WiFi 连接，自动同步显示内容"
        action={
          <Button onClick={() => setAddOpen(true)} iconLeft={<Cpu size={14} />} size="sm">
            添加设备
          </Button>
        }
      >
        {isPending ? (
          <div className="flex justify-center py-8">
            <Spinner label="加载中" />
          </div>
        ) : devices && devices.length > 0 ? (
          <SortableGrid
            sensors={dnd.sensors}
            order={dnd.currentOrder}
            items={dnd.orderedItems}
            onDragEnd={dnd.onDragEnd}
            getKey={(device) => device.id}
            className="grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 fade-up fade-up-1"
            renderItem={renderDevice}
          />
        ) : (
          <EmptyState
            icon={<MonitorSmartphone size={26} />}
            title="尚无设备"
            action={
              <Button onClick={() => setAddOpen(true)} iconLeft={<Cpu size={16} />}>
                添加第一台
              </Button>
            }
          />
        )}
      </Section>

      {openDevice && (
        <DeviceModal
          open
          onOpenChange={(open) => {
            if (!open) closeDeviceModal();
          }}
          device={openDevice}
        />
      )}

      <AddDeviceDialog open={addOpen} onOpenChange={setAddOpen} />
    </>
  );
}
