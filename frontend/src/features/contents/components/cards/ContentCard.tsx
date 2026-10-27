import { memo } from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';
import type { Content } from '@/api';
import { DragHandle } from '@/components/ui/DragHandle';
import { useRefreshDynamicContent } from '@/features/dynamic/query/dynamic-content-queries';
import { useBlob } from '@/features/contents/query/content-image-queries';
import { useMutationAction } from '@/hooks/useMutationAction';
import { AudioPlayPreview } from '../audio/AudioPlayPreview';
import { AudioStatusBadge } from './AudioStatusBadge';
import { ContentCardActions } from './ContentCardActions';
import { ContentCardShell } from './ContentCardShell';
import { useDeleteContentWithConfirm } from '@/features/contents/hooks/useDeleteContentWithConfirm';
import { FrameBitmapPreview } from '@/components/eink/FrameBitmapPreview';
import { useSortableStyle } from '@/components/dnd/useSortableStyle';

interface ContentCardProps {
  gid: string;
  content: Content;
  onEdit: (content: Content) => void;
}

export const ContentCard = memo(function ContentCard({ gid, content, onEdit }: ContentCardProps) {
  const isDynamic = content.kind === 'dynamic';
  const refresh = useRefreshDynamicContent(gid);
  const { deleteWithConfirm, isPending: deletePending } = useDeleteContentWithConfirm({
    gid,
    content,
  });
  const img = useBlob(content.imageHash);
  const { attributes, listeners, setNodeRef, style, isDragging } = useSortableStyle(content.id);
  const refreshContent = useMutationAction<string>({
    isPending: refresh.isPending,
    run: (contentId, callbacks) => refresh.mutate(contentId, callbacks),
    successToast: '已刷新',
    errorToast: '刷新失败',
  });

  return (
    <ContentCardShell
      nodeRef={setNodeRef}
      style={style}
      isDragging={isDragging}
      loading={img.isPending}
      error={!!img.error}
      frameName={content.frameName}
      seq={content.seq}
      preview={
        <FrameBitmapPreview data={img.data} caption={content.statusBarText} showStatusBar={false} />
      }
      topRight={
        <AudioStatusBadge
          status={content.audio?.status ?? null}
          hash={content.audio?.hash ?? null}
          error={isDynamic ? (content.audio?.error ?? null) : undefined}
        />
      }
      titleMeta={
        isDynamic && content.dynamic?.error ? (
          <p
            className="mt-0.5 flex items-center gap-1 truncate font-sans text-[11px] text-clay"
            title={content.dynamic?.error}
          >
            <AlertCircle size={11} className="shrink-0" />
            <span className="truncate">{content.dynamic?.error}</span>
          </p>
        ) : undefined
      }
      actions={
        <ContentCardActions
          dragHandle={<DragHandle attributes={attributes} listeners={listeners} />}
          extraActions={
            isDynamic ? (
              <button
                type="button"
                onClick={() => refreshContent(content.id)}
                aria-label="立即刷新"
                title="立即刷新"
                disabled={refresh.isPending}
                className="p-1.5 text-stone hover:text-ink hover:bg-cream transition-colors disabled:opacity-50"
              >
                <RefreshCw size={14} className={refresh.isPending ? 'animate-spin' : undefined} />
              </button>
            ) : null
          }
          audioPreview={content.audio?.hash ? <AudioPlayPreview hash={content.audio.hash} /> : null}
          onEdit={() => onEdit(content)}
          onDelete={deleteWithConfirm}
          deleteDisabled={deletePending}
        />
      }
    />
  );
});
