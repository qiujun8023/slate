// 内容组卡片：可拖拽排序 + 内容数展示 + 底部操作行。

import { memo } from 'react';
import { Link } from 'react-router-dom';
import { Layers, Trash2 } from 'lucide-react';
import type { Group } from '@/api';
import { DragHandle } from '@/components/ui/DragHandle';
import { IconBlock } from '@/components/ui/IconBlock';
import { formatBytes } from '@/lib/format';
import { cn } from '@/lib/cn';
import { useSortableStyle } from '@/components/dnd/useSortableStyle';
import { appRoutes } from '@/app/routes';

export const GroupCardSortable = memo(function GroupCardSortable({
  group,
  onDelete,
  deleteDisabled,
}: {
  group: Group;
  onDelete: (group: Group) => void;
  deleteDisabled?: boolean;
}) {
  const { attributes, listeners, setNodeRef, style, isDragging } = useSortableStyle(group.id);

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'craft-card flex flex-col overflow-hidden',
        isDragging && 'shadow-drag opacity-90'
      )}
    >
      <Link
        to={appRoutes.group(group.id)}
        className="block flex-1 min-w-0 px-5 py-5 sm:px-6 sm:py-6 hover:bg-cream transition-colors"
      >
        <div className="flex items-start gap-3.5">
          <IconBlock size="lg" tone="soft" title="内容" aria-label="内容">
            <Layers size={24} />
          </IconBlock>
          <div className="min-w-0 flex-1 flex items-center justify-between gap-2">
            <div className="min-w-0">
              <h3 className="font-serif text-[20px] font-bold leading-tight truncate tracking-tight">
                {group.name}
              </h3>
              <p className="font-mono text-[11px] text-stone-light mt-1.5 truncate">
                {formatBytes(group.totalBytes)}
              </p>
            </div>
            <div className="flex items-baseline gap-1 flex-shrink-0">
              <span className="font-serif text-[28px] font-bold leading-none tabular-nums text-ink">
                {group.contentCount}
              </span>
              <span className="font-sans text-[11px] text-stone">项</span>
            </div>
          </div>
        </div>
      </Link>

      <div className="px-2 py-2 border-t border-line flex items-center min-h-[38px]">
        <DragHandle attributes={attributes} listeners={listeners} />

        <span className="flex-1" />

        <button
          type="button"
          disabled={deleteDisabled}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onDelete(group);
          }}
          aria-label="删除"
          title="删除整组"
          className="p-1.5 text-stone hover:text-clay hover:bg-cream transition-colors disabled:opacity-50"
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  );
});
