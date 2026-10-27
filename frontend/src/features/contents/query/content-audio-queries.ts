import { useMutation } from '@tanstack/react-query';
import type { Content, GenerateTTSRequest } from '@/api';
import { http } from '@/lib/http';
import { useInvalidateContentDependencies } from './content-cache-helpers';

export function useDeleteContentAudio(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: (contentId: string) => http.delete<Content>(`/contents/${contentId}/audio`),
    onSuccess: (_data, contentId) => {
      invalidate(contentId);
    },
  });
}

export function useGenerateContentTts(gid: string) {
  const invalidate = useInvalidateContentDependencies(gid);
  return useMutation({
    mutationFn: ({ contentId, body }: { contentId: string; body: GenerateTTSRequest }) =>
      http.post<Content>(`/contents/${contentId}/audio/tts`, body),
    onSuccess: (data) => {
      invalidate(data.id);
    },
  });
}
