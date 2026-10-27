// 统一新建编辑器 — 图片 + 所有动态类型在同一页面切换。
// 仅用于新建；编辑流程仍使用各自的 ImageContentEditor / DynamicContentEditor。

import { useState } from 'react';
import { Plus } from 'lucide-react';
import { DASHBOARD_CUSTOM_STARTER_SAMPLE_DATA } from '@/features/dynamic/model/dashboard-samples';
import { PageHeader } from '@/components/layout/PageHeader';
import { TYPE_META, type AllContentType } from '@/features/contents/model/content-type-meta';
import { DynamicCreateForm } from '@/features/dynamic/components/DynamicCreateForm';
import { DynamicFramePreview } from '@/features/dynamic/components/DynamicFramePreview';
import { useDynamicContentForm } from '@/features/dynamic/hooks/useDynamicContentForm';
import { ContentTypeCardGrid, ContentTypePicker } from './ContentTypePicker';
import { ImageCreateForm } from './ImageCreateForm';

// ─── 主编辑器 ──────────────────────────────────────────────────────────────────

interface ContentCreateEditorProps {
  gid: string;
  onDone: () => void;
  onEditCreatedImage?: (contentId: string) => void;
}

export function ContentCreateEditor({ gid, onDone, onEditCreatedImage }: ContentCreateEditorProps) {
  const dynamicForm = useDynamicContentForm({ requireDashboardData: true });
  const [type, setType] = useState<AllContentType | null>(null);

  function handleTypeChange(t: AllContentType) {
    if (t === type) return;
    setType(t);
    if (t === 'image') {
      dynamicForm.reset();
    } else {
      dynamicForm.loadType(
        t,
        t === 'dashboard' ? { ...DASHBOARD_CUSTOM_STARTER_SAMPLE_DATA } : null
      );
    }
  }

  function resetTypeSelection() {
    setType(null);
    dynamicForm.reset();
  }

  const selectedTypeHeader = type ? (
    <div className="space-y-3">
      <ContentTypePicker value={type} onChange={handleTypeChange} onBack={resetTypeSelection} />
      <p className="font-sans text-[12px] text-stone leading-relaxed">
        {TYPE_META[type].description}
      </p>
      <div className="border-t border-line" />
    </div>
  ) : null;

  return (
    <div>
      <PageHeader
        onBack={onDone}
        icon={<Plus size={24} />}
        title="新建帧"
        subtitle="选择类型后填写参数，创建后追加至列表末尾，可拖拽改序。"
      />

      <div className="mt-6 fade-up fade-up-1">
        {!type ? (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 lg:gap-8">
            <div className="order-2 min-w-0 lg:order-1">
              <p className="font-mono text-[10px] leading-5 text-stone uppercase tracking-[0.18em] ml-0.5 mb-2">
                设备预览
              </p>
              <DynamicFramePreview data={null} pending={false} hasConfig={false} />
            </div>
            <div className="order-1 min-w-0 lg:order-2 lg:mt-7">
              <div className="max-w-3xl">
                <ContentTypeCardGrid onChange={handleTypeChange} />
              </div>
            </div>
          </div>
        ) : type === 'image' ? (
          <ImageCreateForm
            gid={gid}
            header={selectedTypeHeader}
            onDone={onDone}
            onEditCreatedImage={onEditCreatedImage}
          />
        ) : (
          <DynamicCreateForm
            gid={gid}
            type={type}
            form={dynamicForm}
            header={selectedTypeHeader}
            onDone={onDone}
          />
        )}
      </div>
    </div>
  );
}
