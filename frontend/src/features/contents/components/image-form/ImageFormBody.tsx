import type { AudioStatus } from '@/api';
import type { ReactNode } from 'react';
import { FrameHeight, FrameWidth } from '@/api';
import { Input } from '@/components/ui/Input';
import { FormSection } from '@/components/ui/FormSection';
import { cn } from '@/lib/cn';
import { DitherControls } from './DitherControls';
import { ImageAudioBlock } from './ImageAudioBlock';
import { ImageDropzone } from './ImageDropzone';
import { PreviewCanvas } from './PreviewCanvas';
import type { useImageContentForm } from '@/features/contents/hooks/useImageContentForm';

interface ImageFormBodyProps {
  gid: string;
  form: ReturnType<typeof useImageContentForm>;
  isEdit: boolean;
  existingImage?: ArrayBuffer;
  existingImagePending?: boolean;
  hasExistingAudio?: boolean;
  editingContentId?: string | null;
  audioStatus?: AudioStatus | null;
  audioError?: string | null;
  frameNamePlaceholder?: string;
  frameNameAutoFocus?: boolean;
  gridClassName?: string;
  beforeFields?: ReactNode;
  actions: ReactNode;
}

export function ImageFormBody({
  gid,
  form,
  isEdit,
  existingImage,
  existingImagePending,
  hasExistingAudio = false,
  editingContentId = null,
  audioStatus,
  audioError,
  frameNamePlaceholder = '如：挖掘机',
  frameNameAutoFocus,
  gridClassName,
  beforeFields,
  actions,
}: ImageFormBodyProps) {
  return (
    <div
      className={cn('grid grid-cols-1 gap-6 lg:gap-8', gridClassName ?? 'lg:grid-cols-[1.3fr_1fr]')}
    >
      <div className="order-2 min-w-0 lg:order-1">
        <p className="font-mono text-[10px] leading-5 text-stone uppercase tracking-[0.18em] ml-0.5 mb-2">
          预览 · 1bpp · {FrameWidth}×{FrameHeight}
        </p>
        <PreviewCanvas
          canvasRef={form.image.previewRef}
          imageFile={form.image.file}
          existingImage={existingImage}
          existingImagePending={existingImagePending}
          threshold={form.dither.threshold}
          mode={form.dither.mode}
          scale={form.crop.scale}
          offset={form.crop.offset}
          onOffsetChange={form.crop.setOffset}
          statusCaption={form.frameName.trim() || null}
        />
      </div>

      <div className="order-1 min-w-0 lg:order-2 lg:mt-7 space-y-6">
        {beforeFields}

        <FormSection label="帧名称（选填，最多 64 字）">
          <Input
            type="text"
            maxLength={64}
            value={form.frameName}
            onChange={(event) => form.setFrameName(event.target.value)}
            placeholder={frameNamePlaceholder}
            autoFocus={frameNameAutoFocus}
          />
        </FormSection>

        <FormSection label="类型参数" hint={isEdit ? '不传图片则保留原图。' : undefined}>
          <div className="space-y-4">
            <ImageDropzone isEdit={isEdit} imageFile={form.image.file} onPick={form.image.onPick} />
            <DitherControls
              mode={form.dither.mode}
              onModeChange={form.dither.setMode}
              threshold={form.dither.threshold}
              onThresholdChange={form.dither.setThreshold}
              disabled={isEdit && !form.image.file}
              hasImage={!!form.image.file}
              scale={form.crop.scale}
              onScaleChange={form.crop.setScale}
              onResetCrop={form.crop.reset}
            />
          </div>
        </FormSection>

        <FormSection label="音频">
          <ImageAudioBlock
            gid={gid}
            mode={form.audio.mode}
            onModeChange={form.audio.setMode}
            audioFile={form.audio.file}
            onAudioFileChange={form.audio.setFile}
            ttsText={form.audio.ttsText}
            onTtsTextChange={form.audio.setTtsText}
            ttsVoice={form.audio.ttsVoice}
            onTtsVoiceChange={form.audio.setTtsVoice}
            hasExistingAudio={hasExistingAudio}
            editingContentId={editingContentId}
            audioStatus={audioStatus}
            audioError={audioError}
          />
        </FormSection>

        {actions}
      </div>
    </div>
  );
}
