import { useQuery } from '@tanstack/react-query';
import { fetchBinary } from '@/lib/http';
import { contentKeys } from './keys';

/** Blobs are addressed by content hash and never change, so they are cached forever. */
export function useBlob(hash: string | null | undefined) {
  return useQuery({
    queryKey: contentKeys.blob(hash),
    queryFn: () => fetchBinary(`/blobs/${hash}`),
    staleTime: Infinity,
    enabled: !!hash,
  });
}
