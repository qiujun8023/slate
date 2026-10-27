// 组详情：metadata + 内容网格（拖拽排序 + 试听/编辑/删除）+ 顶部「新增」。
//
// dnd-kit reorder 通过 useDndOrder 复用；本地顺序会在保存失败时回滚。

import { Link, useNavigate } from 'react-router-dom';
import { ArrowLeft, Plus, Layers } from 'lucide-react';
import { useGroup, useUpdateGroup } from '@/features/groups/query/group-queries';
import { useGroupContents } from '@/features/contents/query/content-read-queries';
import { useReorderContents } from '@/features/contents/query/content-mutation-queries';
import type { Content, Group } from '@/api';
import { Spinner } from '@/components/ui/Spinner';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { SortableGrid } from '@/components/dnd/SortableGrid';
import { ContentCard } from '@/features/contents/components/cards/ContentCard';
import { PageHeader } from '@/components/layout/PageHeader';
import { RequireRouteParams } from '@/components/layout/RequireRouteParams';
import { InlineRename } from '@/components/ui/InlineRename';
import { useInlineRename } from '@/hooks/useInlineRename';
import { useToast } from '@/components/feedback/toast-context';
import { getApiErrorMessage } from '@/lib/api-errors';
import { formatBytes } from '@/lib/format';
import { useDndOrder } from '@/components/dnd/useDndOrder';
import { appRoutes } from '@/app/routes';

export function GroupDetailPage() {
  const navigate = useNavigate();

  return (
    <RequireRouteParams
      names={['gid'] as const}
      hint="请从总览页进入具体内容组。"
      action={<BackHomeLink />}
    >
      {({ gid }) => <GroupDetailContent key={gid} gid={gid} navigate={navigate} />}
    </RequireRouteParams>
  );
}

function GroupDetailContent({
  gid,
  navigate,
}: {
  gid: string;
  navigate: ReturnType<typeof useNavigate>;
}) {
  const groupQuery = useGroup(gid);
  const contents = useGroupContents(gid);
  const reorder = useReorderContents(gid);
  const toast = useToast();

  const group = groupQuery.data;
  const { sensors, currentOrder, orderedItems, onDragEnd } = useDndOrder(
    contents.data,
    getContentId,
    (newOrder) =>
      reorder.mutateAsync({ order: newOrder }).catch((err: unknown) => {
        toast.error('排序保存失败', getApiErrorMessage(err));
        throw err;
      })
  );

  const openCreate = () => navigate(appRoutes.newContent(gid));
  const openEdit = (content: Content) => {
    if (content.kind === 'dynamic') {
      navigate(appRoutes.editDynamicContent(gid, content.id));
    } else {
      navigate(appRoutes.editImageContent(gid, content.id));
    }
  };
  const goBack = () => {
    navigate(appRoutes.home);
  };

  if (groupQuery.isPending) {
    return (
      <div className="pt-16 text-center">
        <Spinner label="加载中" />
      </div>
    );
  }
  if (groupQuery.isError || !group) {
    return (
      <EmptyState
        title="内容不存在或已被删除"
        action={
          <Link
            to={appRoutes.home}
            className="inline-flex items-center gap-1 text-[13px] text-stone border-b border-stone"
          >
            <ArrowLeft size={13} /> 返回总览
          </Link>
        }
      />
    );
  }

  return (
    <div>
      <GroupHeader group={group} onBack={goBack} onAdd={openCreate} />
      <div className="mt-6 fade-up fade-up-1">
        {contents.isPending ? (
          <Spinner label="加载中" />
        ) : contents.isError ? (
          <EmptyState title="加载失败" hint="请刷新重试。" />
        ) : contents.data && contents.data.length > 0 ? (
          <SortableGrid
            sensors={sensors}
            order={currentOrder}
            items={orderedItems}
            onDragEnd={onDragEnd}
            getKey={(content) => content.id}
            className="grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4"
            renderItem={(content) => <ContentCard gid={gid} content={content} onEdit={openEdit} />}
          />
        ) : (
          <EmptyState
            title="尚无内容"
            hint="点击新建帧开始添加内容。"
            action={
              <Button onClick={openCreate} iconLeft={<Plus size={16} />}>
                新建帧
              </Button>
            }
          />
        )}
      </div>
    </div>
  );
}

function getContentId(content: Content): string {
  return content.id;
}

function BackHomeLink() {
  return (
    <Link
      to={appRoutes.home}
      className="inline-flex items-center gap-1 text-[13px] text-stone border-b border-stone"
    >
      <ArrowLeft size={13} /> 返回总览
    </Link>
  );
}

// ───── 组标题 + inline 改名 + 新建内容 ───────────────────────────
function GroupHeader({
  group,
  onBack,
  onAdd,
}: {
  group: Group;
  onBack: () => void;
  onAdd: () => void;
}) {
  const update = useUpdateGroup(group.id);
  const toast = useToast();

  const { editing, draft, setDraft, startEditing, commit, handleKeyDown } = useInlineRename(
    group.name,
    async (name) => {
      try {
        await update.mutateAsync({ name });
        toast.success('已改名');
      } catch (err) {
        toast.error('改名失败', getApiErrorMessage(err));
        throw err;
      }
    }
  );

  return (
    <PageHeader
      backLabel="总览"
      onBack={onBack}
      icon={<Layers size={24} />}
      title={group.name}
      titleContent={
        <InlineRename
          editing={editing}
          value={group.name}
          draft={draft}
          onDraftChange={setDraft}
          onStart={startEditing}
          onCommit={commit}
          onKeyDown={handleKeyDown}
          pending={update.isPending}
          titleClassName="font-serif text-[32px] sm:text-[40px] font-bold leading-[1.2] truncate tracking-tight"
          inputClassName="!font-serif !font-bold !text-[32px] sm:!text-[40px] !leading-[1.2]"
          buttonClassName="p-2 -m-1"
        />
      }
      subtitle={`${group.contentCount} 项 · ${formatBytes(group.totalBytes)}`}
      action={
        <Button iconLeft={<Plus size={16} />} size="sm" onClick={onAdd}>
          新建帧
        </Button>
      }
    />
  );
}
