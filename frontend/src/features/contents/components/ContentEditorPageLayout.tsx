import { useCallback, type ReactNode } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import type { Content } from '@/api';
import { useContentDetail } from '@/features/contents/query/content-read-queries';
import { EmptyState } from '@/components/ui/EmptyState';
import { RequireRouteParams } from '@/components/layout/RequireRouteParams';
import { Spinner } from '@/components/ui/Spinner';
import { appRoutes } from '@/app/routes';

interface ContentEditorPageLayoutProps {
  missingContentHint: string;
  notFoundTitle: string;
  findContent: (content: Content) => boolean;
  renderEditor: (props: { gid: string; content: Content; onDone: () => void }) => ReactNode;
}

export function ContentEditorPageLayout({
  missingContentHint,
  notFoundTitle,
  findContent,
  renderEditor,
}: ContentEditorPageLayoutProps) {
  return (
    <RequireRouteParams names={['gid'] as const} hint="请从总览页进入具体内容组。">
      {({ gid }) => (
        <ContentEditorPageContent
          gid={gid}
          missingContentHint={missingContentHint}
          notFoundTitle={notFoundTitle}
          findContent={findContent}
          renderEditor={renderEditor}
        />
      )}
    </RequireRouteParams>
  );
}

function ContentEditorPageContent({
  gid,
  missingContentHint,
  notFoundTitle,
  findContent,
  renderEditor,
}: ContentEditorPageLayoutProps & { gid: string }) {
  const { contentId } = useParams();
  const navigate = useNavigate();
  const content = useContentDetail(contentId);

  const onDone = useCallback(() => {
    navigate(appRoutes.group(gid));
  }, [gid, navigate]);

  if (!contentId) {
    return <EmptyState title="页面不存在" hint={missingContentHint} />;
  }

  if (content.isPending) {
    return (
      <div className="pt-16 text-center">
        <Spinner label="加载中" />
      </div>
    );
  }

  if (content.isError) {
    return <EmptyState title="加载失败" hint="请刷新重试。" />;
  }

  const isExpectedContent = content.data ? findContent(content.data) : false;

  if (content.data && content.data.id !== contentId) {
    return <EmptyState title="加载失败" hint="内容详情返回了不匹配的 ID，请刷新重试。" />;
  }

  if (!content.data || !isExpectedContent || content.data.groupId !== gid) {
    if (content.isFetching) {
      return (
        <div className="pt-16 text-center">
          <Spinner label="加载中" />
        </div>
      );
    }
    return <EmptyState title={notFoundTitle} />;
  }

  return <>{renderEditor({ gid, content: content.data, onDone })}</>;
}
