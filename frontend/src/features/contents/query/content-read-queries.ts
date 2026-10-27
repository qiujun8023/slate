import { useQuery } from '@tanstack/react-query';
import type { Content } from '@/api';
import { http } from '@/lib/http';
import { audioGenerationRefetchInterval } from './content-cache-helpers';
import { contentKeys } from './keys';

export function useGroupContents(gid: string | undefined) {
  return useQuery({
    queryKey: contentKeys.group(gid),
    queryFn: () => http.get<Content[]>(`/groups/${gid}/contents`),
    enabled: !!gid,
    refetchInterval: audioGenerationRefetchInterval,
  });
}

export function useContentDetail(contentId: string | undefined) {
  return useQuery({
    queryKey: contentKeys.detail(contentId),
    queryFn: () => http.get<Content>(`/contents/${contentId}`),
    enabled: !!contentId,
    refetchInterval: audioGenerationRefetchInterval,
  });
}
