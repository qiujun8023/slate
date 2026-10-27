// 图片内容编辑器 — 仅用于编辑已有图片内容；新建流程走 ContentCreateEditor。
//
// 拆分：
//   PreviewCanvas   — 1bpp 预览 + 拖拽/缩放交互
//   ImageDropzone   — 选图(可选)
//   AudioDropzone   — 选音频 + 删除已有音频
//   DitherControls  — 缩放 / 抖动算法 / 阈值

import type { FormEvent } from 'react';
import { Image as ImageIcon } from 'lucide-react';
import type { Content } from '@/api';
import { useGenerateContentTts } from '@/features/contents/query/content-audio-queries';
import { useBlob } from '@/features/contents/query/content-image-queries';
import {
  usePatchContentFrameName,
  useUpdateImageContent,
} from '@/features/contents/query/content-mutation-queries';
import { useToast } from '@/components/feedback/toast-context';
import { FormActions } from '@/components/ui/FormActions';
import { PageHeader } from '@/components/layout/PageHeader';
import { TYPE_META } from '@/features/contents/model/content-type-meta';
import { getApiErrorMessage } from '@/lib/api-errors';
import { useImageContentForm } from '@/features/contents/hooks/useImageContentForm';
import { ImageFormBody } from './ImageFormBody';

interface ImageContentEditorProps {
  gid: string;
  content: Content;
  onDone: () => void;
}

export function ImageContentEditor({ gid, content, onDone }: ImageContentEditorProps) {
  const updateImageContent = useUpdateImageContent(gid);
  const patchFrameName = usePatchContentFrameName(gid);
  const generateTts = useGenerateContentTts(gid);
  const submitting =
    updateImageContent.isPending || patchFrameName.isPending || generateTts.isPending;
  const toast = useToast();
  const form = useImageContentForm(content);

  const existingImg = useBlob(!form.image.file ? content.imageHash : null);
  const canSubmit = form.canEdit;

  async function submitContent() {
    try {
      if (form.hasFilePatch) {
        const fd = await form.buildFormData();
        await updateImageContent.mutateAsync({ contentId: content.id, form: fd });
      } else if (form.frameNameChanged) {
        await patchFrameName.mutateAsync({
          contentId: content.id,
          frameName: form.frameName.trim() || null,
        });
      }
      if (form.audio.wantsTts) {
        try {
          await generateTts.mutateAsync({
            contentId: content.id,
            body: { text: form.audio.trimmedTtsText, voice: form.audio.ttsVoice },
          });
        } catch (err) {
          const title = form.hasContentPatch ? '内容已保存，TTS 生成失败' : 'TTS 生成失败';
          toast.error(title, `${getApiErrorMessage(err)}。可调整 TTS 文案后重新保存。`);
          return;
        }
      }
      toast.success('内容已保存');
      onDone();
    } catch (err) {
      toast.error('保存失败', getApiErrorMessage(err));
    }
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void submitContent();
  }

  return (
    <div>
      <PageHeader
        onBack={onDone}
        icon={<ImageIcon size={24} />}
        title={`编辑第 ${content.seq + 1} 项`}
        subtitle="改顺序请在组内用拖拽。"
      />

      <div className="mt-6 fade-up fade-up-1">
        <form onSubmit={onSubmit}>
          <ImageFormBody
            gid={gid}
            form={form}
            isEdit
            existingImage={existingImg.data}
            existingImagePending={existingImg.isPending && !form.image.file}
            hasExistingAudio={!!content.audio?.hash}
            editingContentId={content.id}
            audioStatus={content.audio?.status ?? null}
            audioError={content.audio?.error ?? null}
            beforeFields={
              <div className="space-y-3">
                <p className="font-sans text-[12px] text-stone leading-relaxed">
                  {TYPE_META.image.description}
                </p>
                <div className="border-t border-line" />
              </div>
            }
            actions={
              <FormActions
                onCancel={onDone}
                submitLabel="保存"
                disabled={!canSubmit}
                submitting={submitting}
              />
            }
          />
        </form>
      </div>
    </div>
  );
}
