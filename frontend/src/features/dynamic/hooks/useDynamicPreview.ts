import { useCallback, useEffect, useRef, useState } from 'react';
import type { DynamicConfig, DynamicType } from '@/api';
import { usePreviewDynamicContent } from '@/features/dynamic/query/dynamic-content-queries';

type PreviewContentType = DynamicType | 'image';

export function useDynamicPreview({
  contentId,
  type,
  config,
  dashboardData,
  debounceMs = 800,
}: {
  contentId?: string;
  type: PreviewContentType | null;
  config: DynamicConfig | null;
  dashboardData?: Record<string, unknown> | null;
  debounceMs?: number;
}) {
  const preview = usePreviewDynamicContent(contentId);
  const [livePreviewData, setLivePreviewData] = useState<ArrayBuffer | null>(null);
  const previewSeq = useRef(0);
  const previewMutate = preview.mutate;

  const invalidatePreview = useCallback(() => {
    previewSeq.current++;
    setLivePreviewData(null);
  }, []);

  useEffect(() => {
    if (!config || type === 'image' || type === null) {
      invalidatePreview();
      return;
    }
    const seq = ++previewSeq.current;
    setLivePreviewData(null);
    const controller = new AbortController();
    const timer = setTimeout(() => {
      previewMutate(
        {
          config,
          data: config.type === 'dashboard' ? (dashboardData ?? undefined) : undefined,
          signal: controller.signal,
        },
        {
          onSuccess: (data) => {
            if (seq === previewSeq.current) setLivePreviewData(data);
          },
          onError: () => {
            if (seq === previewSeq.current) setLivePreviewData(null);
          },
        }
      );
    }, debounceMs);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [config, dashboardData, debounceMs, invalidatePreview, previewMutate, type]);

  return { livePreviewData, previewPending: preview.isPending, invalidatePreview };
}
