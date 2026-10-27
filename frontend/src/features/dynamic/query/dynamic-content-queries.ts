import { useMutation } from '@tanstack/react-query';
import type {
  Content,
  CreateDynamicContentRequest,
  DynamicConfig,
  PatchContentRequest,
  PreviewDynamicRequest,
} from '@/api';
import { api, http, SLOW_REQUEST } from '@/lib/http';
import { useInvalidateContentDependencies } from '@/features/contents/query/content-cache-helpers';

export function useCreateDynamicContent(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: (body: CreateDynamicContentRequest) =>
      http.post<Content>(`/groups/${gid}/contents`, body, SLOW_REQUEST),
    onSuccess: (data) => {
      invalidate(data.id);
    },
  });
}

export function useUpdateDynamicContent(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: ({
      contentId,
      frameName,
      config,
    }: {
      contentId: string;
      frameName?: string | null;
      config?: DynamicConfig;
    }) => {
      const body: PatchContentRequest = {};
      if (frameName !== undefined) body.frameName = frameName;
      if (config !== undefined) body.config = config;
      return http.patch<Content>(`/contents/${contentId}`, body, SLOW_REQUEST);
    },
    onSuccess: (data) => {
      invalidate(data.id);
    },
  });
}

export function useRefreshDynamicContent(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: (contentId: string) =>
      http.post<Content>(`/contents/${contentId}/refresh`, undefined, SLOW_REQUEST),
    onSuccess: (_data, contentId) => {
      invalidate(contentId);
    },
  });
}

export function usePreviewDynamicContent(contentId: string | undefined) {
  return useMutation({
    mutationFn: async ({
      config,
      data: previewData,
      signal,
    }: {
      config: DynamicConfig;
      data?: Record<string, unknown>;
      signal?: AbortSignal;
    }) => {
      const url = contentId ? `/contents/${contentId}/preview` : '/contents/preview';
      const body: PreviewDynamicRequest = { config, data: previewData };
      const { data } = await api.post<ArrayBuffer>(url, body, {
        ...SLOW_REQUEST,
        responseType: 'arraybuffer',
        signal,
      });
      return data;
    },
  });
}
