// 动态内容编辑页 —— create + edit 共用。
//
// 路由：
//   /groups/:gid/contents/dynamic/:contentId/edit — 编辑

import { DynamicContentEditor } from '@/features/dynamic/components/DynamicContentEditor';
import { EmptyState } from '@/components/ui/EmptyState';
import { Button } from '@/components/ui/Button';
import { ContentEditorPageLayout } from '@/features/contents/components/ContentEditorPageLayout';

export function DynamicContentEditorPage() {
  return (
    <ContentEditorPageLayout
      missingContentHint="请从内容列表进入动态内容编辑页。"
      notFoundTitle="动态内容不存在或已删除"
      findContent={(content) => content.kind === 'dynamic'}
      renderEditor={({ gid, content, onDone }) => {
        if (!content.dynamic?.type || !content.dynamic?.config) {
          return (
            <EmptyState
              title="动态内容配置缺失"
              hint="这条动态内容的数据不完整，请返回内容列表后重试。"
              action={
                <Button variant="outline" size="sm" onClick={onDone}>
                  返回
                </Button>
              }
            />
          );
        }

        // ContentDetail 已经带 dynamic_type / dynamic_config，省一次 GET /contents/:id 请求。
        return (
          <DynamicContentEditor
            gid={gid}
            content={content}
            initialType={content.dynamic?.type}
            initialConfig={content.dynamic?.config}
            onDone={onDone}
          />
        );
      }}
    />
  );
}
