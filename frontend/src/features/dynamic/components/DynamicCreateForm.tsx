import type { FormEvent, ReactNode } from 'react';
import { isAudioDynamicConfig, type DynamicType } from '@/api';
import { useToast } from '@/components/feedback/toast-context';
import { FormActions } from '@/components/ui/FormActions';
import { useCreateDynamicContent } from '@/features/dynamic/query/dynamic-content-queries';
import { DynamicContentFields } from '@/features/dynamic/components/DynamicContentFields';
import { DynamicContentFormShell } from '@/features/dynamic/components/DynamicContentFormShell';
import { DynamicFramePreview } from '@/features/dynamic/components/DynamicFramePreview';
import { useDynamicContentForm } from '@/features/dynamic/hooks/useDynamicContentForm';
import { DYNAMIC_TYPE_META } from '@/features/dynamic/model/type-meta';
import { getApiErrorMessage } from '@/lib/api-errors';

interface DynamicCreateFormProps {
  gid: string;
  type: DynamicType;
  form: ReturnType<typeof useDynamicContentForm>;
  header?: ReactNode;
  onDone: () => void;
}

export function DynamicCreateForm({ gid, type, form, header, onDone }: DynamicCreateFormProps) {
  const createDynamic = useCreateDynamicContent(gid);
  const toast = useToast();
  const dynamicMeta = DYNAMIC_TYPE_META[type];
  const showDynamicParams = Boolean(dynamicMeta?.hasConfigurableParams);
  const showDynamicAudio = Boolean(
    dynamicMeta?.supportsAudio && form.config && isAudioDynamicConfig(form.config)
  );

  async function submitContent() {
    const parsed = form.submitConfig();
    if (!parsed.ok) {
      toast.error('配置有误', parsed.error);
      return;
    }
    try {
      await createDynamic.mutateAsync({
        config: parsed.config,
        frameName: parsed.frameName,
        initialData: parsed.dashboardData,
      });
      toast.success('已创建');
      onDone();
    } catch (err) {
      toast.error('创建失败', getApiErrorMessage(err));
    }
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void submitContent();
  }

  return (
    <DynamicContentFormShell
      onSubmit={onSubmit}
      gridClassName="lg:grid-cols-2"
      preview={
        <DynamicFramePreview
          data={form.livePreviewData}
          pending={form.previewPending}
          hasConfig={!!form.config}
          caption={form.caption}
        />
      }
      header={header}
      fields={
        form.config ? (
          <DynamicContentFields
            type={type}
            config={form.config}
            frameName={form.frameName}
            onFrameNameChange={form.setFrameName}
            onConfigChange={form.changeConfig}
            showParams={showDynamicParams}
            showAudio={showDynamicAudio}
            dashboardData={form.dashboardData}
            onDashboardDataChange={form.setDashboardData}
            dashboardDataLabel="初始数据 JSON"
          />
        ) : null
      }
      actions={
        <FormActions
          onCancel={onDone}
          submitLabel="创建"
          disabled={!form.canSubmit}
          submitting={createDynamic.isPending}
        />
      }
    />
  );
}
