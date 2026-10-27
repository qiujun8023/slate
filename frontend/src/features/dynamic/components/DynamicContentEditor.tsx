// 动态内容编辑器 —— 编辑动态内容配置。

import { useCallback, useMemo, type FormEvent } from 'react';
import { Sparkles } from 'lucide-react';
import { isAudioDynamicConfig, type Content, type DynamicConfig, type DynamicType } from '@/api';
import { useUpdateDynamicContent } from '@/features/dynamic/query/dynamic-content-queries';
import { useBlob } from '@/features/contents/query/content-image-queries';
import { useToast } from '@/components/feedback/toast-context';
import { FormActions } from '@/components/ui/FormActions';
import { PageHeader } from '@/components/layout/PageHeader';
import { getApiErrorMessage } from '@/lib/api-errors';
import { DYNAMIC_TYPE_META } from '@/features/dynamic/model/type-meta';
import { useDynamicEditorBaselineSync } from '@/features/dynamic/hooks/useDynamicEditorBaselineSync';
import { SavedOrLiveDynamicFramePreview } from '@/features/dynamic/components/DynamicFramePreview';
import { useDynamicContentForm } from '@/features/dynamic/hooks/useDynamicContentForm';
import { DynamicContentFormShell } from './DynamicContentFormShell';
import { DynamicContentFields } from './DynamicContentFields';

interface DynamicContentEditorProps {
  gid: string;
  content: Content;
  initialConfig: DynamicConfig;
  initialType: DynamicType;
  onDone: () => void;
}

export function DynamicContentEditor({
  gid,
  content,
  initialConfig,
  initialType,
  onDone,
}: DynamicContentEditorProps) {
  const update = useUpdateDynamicContent(gid);
  const submitting = update.isPending;
  const toast = useToast();
  const initialDashboardData = useMemo(
    () => (initialType === 'dashboard' ? dashboardDataRecord(content.dynamic?.data) : null),
    [content.dynamic?.data, initialType]
  );
  const form = useDynamicContentForm({
    contentId: content.id,
    initialType,
    initialConfig,
    initialFrameName: content.frameName,
    initialDashboardData,
  });
  const { baseline, setBaseline } = useDynamicEditorBaselineSync({
    contentId: content.id,
    serverType: initialType,
    serverFrameName: content.frameName,
    serverConfig: initialConfig,
    type: form.type,
    frameName: form.frameName,
    configKey: form.configKey,
    setType: form.setType,
    setFrameName: form.setFrameName,
    setConfig: form.setConfig,
  });
  const dirty = useMemo(
    () =>
      form.type !== baseline.type ||
      form.frameName !== baseline.frameName ||
      form.configKey !== baseline.configKey,
    [
      baseline.configKey,
      baseline.frameName,
      baseline.type,
      form.configKey,
      form.frameName,
      form.type,
    ]
  );

  const savedPreviewEnabled = !!content.imageHash;
  const savedPreview = useBlob(content.imageHash);
  const dynamicMeta = form.type ? DYNAMIC_TYPE_META[form.type] : null;
  const showParams = Boolean(dynamicMeta?.hasConfigurableParams);
  const showAudio = Boolean(
    dynamicMeta?.supportsAudio && form.config && isAudioDynamicConfig(form.config)
  );

  const onSubmit = useCallback(async () => {
    const parsed = form.submitConfig();
    if (!parsed.ok) {
      toast.error('配置有误', parsed.error);
      return;
    }
    try {
      await update.mutateAsync({
        contentId: content.id,
        frameName: parsed.frameName,
        config: parsed.config,
      });
      setBaseline({
        contentId: content.id,
        type: parsed.type,
        frameName: form.frameName,
        configKey: form.configKey,
      });
      toast.success('已保存');
      onDone();
    } catch (err) {
      toast.error('保存失败', getApiErrorMessage(err));
    }
  }, [content.id, form, onDone, setBaseline, toast, update]);

  const handleSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      void onSubmit();
    },
    [onSubmit]
  );

  return (
    <div>
      <PageHeader
        onBack={onDone}
        icon={<Sparkles size={24} />}
        title="编辑动态内容"
        subtitle="动态内容由服务端生成 400×300 1bpp 帧，设备端直接显示并叠加状态栏。"
      />

      <div className="mt-6 fade-up fade-up-1">
        <DynamicContentFormShell
          onSubmit={handleSubmit}
          gridClassName="lg:grid-cols-[1.3fr_1fr]"
          preview={
            <SavedOrLiveDynamicFramePreview
              savedData={savedPreview.data}
              savedPending={savedPreviewEnabled && savedPreview.isPending}
              liveData={form.livePreviewData}
              livePending={form.previewPending}
              hasConfig={!!form.config}
              caption={form.caption}
            />
          }
          header={
            <div className="space-y-3">
              <p className="font-sans text-[12px] text-stone leading-relaxed">
                {form.type ? DYNAMIC_TYPE_META[form.type].description : ''}
              </p>
              <div className="border-t border-line" />
            </div>
          }
          fields={
            form.type && form.config ? (
              <DynamicContentFields
                type={form.type}
                config={form.config}
                frameName={form.frameName}
                onFrameNameChange={form.setFrameName}
                onConfigChange={form.changeConfig}
                showParams={showParams}
                showAudio={showAudio}
                contentId={content.id}
                dashboardData={form.dashboardData}
                dashboardDataLabel="当前数据 JSON"
              />
            ) : null
          }
          actions={
            <FormActions
              onCancel={onDone}
              submitLabel="保存"
              disabled={!dirty}
              submitting={submitting}
            />
          }
        />
      </div>
    </div>
  );
}

function dashboardDataRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
