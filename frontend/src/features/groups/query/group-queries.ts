import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Group, GroupRequest, OrderRequest } from '@/api';
import { http } from '@/lib/http';
import { deviceKeys } from '@/features/devices/query/keys';
import { groupKeys } from './keys';

export function useGroups() {
  return useQuery({
    queryKey: groupKeys.list,
    queryFn: async () => {
      return http.get<Group[]>(`/groups`);
    },
    staleTime: 10_000,
  });
}

export function useGroup(gid: string | undefined) {
  return useQuery({
    queryKey: groupKeys.detail(gid),
    queryFn: async () => {
      return http.get<Group>(`/groups/${gid}`);
    },
    enabled: !!gid,
    staleTime: 10_000,
  });
}

export function useCreateGroup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: GroupRequest) => {
      return http.post<Group>(`/groups`, body);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: groupKeys.list }),
  });
}

export function useUpdateGroup(gid: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: GroupRequest) => {
      return http.patch<Group>(`/groups/${gid}`, body);
    },
    onSuccess: (updated) => {
      qc.setQueryData(groupKeys.detail(updated.id), updated);
      qc.setQueryData<Group[]>(groupKeys.list, (groups) => {
        if (!groups) return groups;
        return groups.map((group) => (group.id === updated.id ? updated : group));
      });
    },
  });
}

export function useReorderGroups() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: OrderRequest) => {
      await http.put(`/groups/order`, body);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: groupKeys.list }),
  });
}

export function useDeleteGroup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (gid: string) => {
      await http.delete(`/groups/${gid}`);
    },
    onSuccess: (_data, gid) => {
      qc.removeQueries({ queryKey: groupKeys.detail(gid) });
      qc.invalidateQueries({ queryKey: groupKeys.list });
      qc.invalidateQueries({ queryKey: deviceKeys.list });
    },
  });
}
