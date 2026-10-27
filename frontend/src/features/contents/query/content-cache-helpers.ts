import { useCallback } from 'react';
import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import type { Content } from '@/api';
import { groupKeys } from '@/features/groups/query/keys';
import { contentKeys } from './keys';

const AUDIO_GENERATING_REFETCH_INTERVAL_MS = 2500;

type AudioStatusRow = Pick<Content, 'audio'>;
type AudioStatusQueryData = AudioStatusRow | AudioStatusRow[];

export function audioGenerationRefetchInterval(query: { state: { data?: AudioStatusQueryData } }) {
  const data = query.state.data;
  const hasGeneratingAudio = Array.isArray(data)
    ? data.some(isGeneratingAudio)
    : isGeneratingAudio(data);
  return hasGeneratingAudio ? AUDIO_GENERATING_REFETCH_INTERVAL_MS : false;
}

export function useInvalidateContentDependencies(gid: string) {
  const qc = useQueryClient();
  return useCallback(
    (contentId?: string) => invalidateContentDependencies(qc, gid, contentId),
    [gid, qc]
  );
}

function isGeneratingAudio(row: AudioStatusRow | undefined): boolean {
  return row?.audio?.status === 'pending' || row?.audio?.status === 'generating';
}

function invalidateContentDependencies(qc: QueryClient, gid: string, contentId?: string) {
  void qc.invalidateQueries({ queryKey: contentKeys.group(gid) });
  void qc.invalidateQueries({ queryKey: groupKeys.list });
  void qc.invalidateQueries({ queryKey: groupKeys.detail(gid) });
  if (contentId) {
    void qc.invalidateQueries({ queryKey: contentKeys.detail(contentId) });
  }
}
