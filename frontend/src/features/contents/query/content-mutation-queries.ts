import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { Content, OrderRequest } from '@/api';
import { http } from '@/lib/http';
import { useInvalidateContentDependencies } from './content-cache-helpers';
import { contentKeys } from './keys';

export function useCreateImageContent(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: async (form: FormData) => {
      return http.post<Content>(`/groups/${gid}/contents`, form);
    },
    onSuccess: (data) => {
      invalidate(data.id);
    },
  });
}

export function useUpdateImageContent(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: async ({ contentId, form }: { contentId: string; form: FormData }) => {
      return http.patch<Content>(`/contents/${contentId}`, form);
    },
    onSuccess: (data) => {
      invalidate(data.id);
    },
  });
}

export function usePatchContentFrameName(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: async ({
      contentId,
      frameName,
    }: {
      contentId: string;
      frameName: string | null;
    }) => {
      return http.patch<Content>(`/contents/${contentId}`, { frameName });
    },
    onSuccess: (data) => {
      invalidate(data.id);
    },
  });
}

export function useDeleteContent(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: async (contentId: string) => {
      await http.delete(`/contents/${contentId}`);
      return contentId;
    },
    onSuccess: (_data, contentId) => {
      invalidate(contentId);
    },
  });
}

export function useReorderContents(gid: string) {
  const qc = useQueryClient();
  const invalidate = useInvalidateContentDependencies(gid);
  const groupKey = contentKeys.group(gid);
  return useMutation({
    mutationFn: async (body: OrderRequest) => {
      await http.put(`/groups/${gid}/contents/order`, body);
    },
    // 乐观更新 seq：避免拖拽落点到 refetch 完成之间 ContentCard 显示旧序号。
    onMutate: async ({ order }) => {
      await qc.cancelQueries({ queryKey: groupKey });
      const previous = qc.getQueryData<Content[]>(groupKey);
      if (previous) {
        const byId = new Map(previous.map((c) => [c.id, c]));
        const reordered: Content[] = order
          .map((id, idx) => {
            const item = byId.get(id);
            return item ? { ...item, seq: idx } : undefined;
          })
          .filter((c): c is Content => c !== undefined);
        qc.setQueryData<Content[]>(groupKey, reordered);
      }
      return { previous };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) qc.setQueryData(groupKey, ctx.previous);
    },
    onSettled: () => {
      invalidate();
    },
  });
}
