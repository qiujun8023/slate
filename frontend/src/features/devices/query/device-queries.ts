import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { ClaimDeviceRequest, Device, PatchDeviceRequest, OrderRequest } from '@/api';
import { http } from '@/lib/http';
import { groupKeys } from '@/features/groups/query/keys';
import { deviceKeys } from './keys';

export function useDevices() {
  return useQuery({
    queryKey: deviceKeys.list,
    queryFn: async () => {
      return http.get<Device[]>(`/devices`);
    },
    refetchInterval: 30_000,
    staleTime: 10_000,
  });
}

export function useClaimByPairCode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: ClaimDeviceRequest) => {
      return http.post<Device>(`/devices/claims`, body);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: deviceKeys.list }),
  });
}

export function useReorderDevices() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: OrderRequest) => {
      await http.put(`/devices/order`, body);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: deviceKeys.list }),
  });
}

export function useUnbindDevice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (deviceId: string) => {
      await http.delete(`/devices/${deviceId}/binding`);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: deviceKeys.list }),
  });
}

export function usePatchDevice(deviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: PatchDeviceRequest) => {
      await http.patch(`/devices/${deviceId}`, body);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: deviceKeys.list });
      qc.invalidateQueries({ queryKey: groupKeys.list });
    },
  });
}
