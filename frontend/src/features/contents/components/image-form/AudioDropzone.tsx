// Mono Press 音频 dropzone — 虚线框，0 圆角。

import { useCallback } from 'react';
import { useDropzone } from 'react-dropzone';
import { Music, Trash2 } from 'lucide-react';
import { useDeleteContentAudio } from '@/features/contents/query/content-audio-queries';
import { useConfirmAction } from '@/hooks/useMutationAction';
import { cn } from '@/lib/cn';
import { formatBytes } from '@/lib/format';

interface AudioDropzoneProps {
  gid: string;
  hasExistingAudio: boolean;
  editingContentId: string | null;
  audioFile: File | null;
  onPick: (f: File | null) => void;
  disabled?: boolean;
  /** 由外层 FormSection 提供 label 时关闭组件自带 label。 */
  hideLabel?: boolean;
}

export function AudioDropzone({
  gid,
  hasExistingAudio,
  editingContentId,
  audioFile,
  onPick,
  disabled,
  hideLabel,
}: AudioDropzoneProps) {
  const { mutate: deleteAudioMutate, isPending: deleteAudioPending } = useDeleteContentAudio(gid);
  const deleteAudioWithConfirm = useConfirmAction<string>({
    isPending: deleteAudioPending,
    getConfirmOptions: useCallback(
      () => ({
        title: '删除这一帧的音频？',
        description: '图保留，只移除音频文件。',
        destructive: true,
        confirmText: '删除音频',
      }),
      []
    ),
    run: useCallback(
      (contentId, callbacks) => deleteAudioMutate(contentId, callbacks),
      [deleteAudioMutate]
    ),
    successToast: '音频已删除',
    errorToast: '删除失败',
  });

  const dz = useDropzone({
    onDrop: (files) => onPick(files[0] ?? null),
    accept: {
      'audio/*': ['.wav', '.mp3', '.aac', '.m4a', '.ogg', '.opus', '.flac', '.wma'],
    },
    maxFiles: 1,
    disabled,
  });

  const deleteAudio =
    hasExistingAudio && editingContentId != null ? (
      <DeleteAudioButton
        isPending={deleteAudioPending}
        label={hideLabel ? '删除已有音频' : '删除'}
        onDelete={() => deleteAudioWithConfirm(editingContentId)}
      />
    ) : null;

  return (
    <div>
      {!hideLabel && (
        <div className="flex items-center justify-between mb-2">
          <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em]">
            音频{hasExistingAudio ? ' · 已有' : ' · 选填'}
          </p>
          {deleteAudio}
        </div>
      )}
      {hideLabel && deleteAudio && <div className="flex justify-end mb-2">{deleteAudio}</div>}
      <div
        {...dz.getRootProps()}
        className={cn(
          'border border-dashed px-4 py-3 cursor-pointer transition-colors flex items-center gap-3',
          disabled
            ? 'border-line opacity-50 cursor-not-allowed'
            : dz.isDragActive
              ? 'border-ink bg-cream'
              : 'border-ink/50 hover:border-ink hover:bg-cream'
        )}
      >
        <input {...dz.getInputProps()} />
        <Music size={16} className="text-stone-light flex-shrink-0" />
        <div className="min-w-0 flex-1">
          {audioFile ? (
            <>
              <p className="font-mono text-[12px] text-ink truncate">{audioFile.name}</p>
              <p className="font-sans text-[11px] text-stone">{formatBytes(audioFile.size)}</p>
            </>
          ) : hasExistingAudio ? (
            <p className="font-sans text-[13px] text-stone">拖新文件可替换</p>
          ) : (
            <p className="font-sans text-[13px] text-stone">
              MP3 / WAV / OGG / FLAC / AAC，自动转码
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function DeleteAudioButton({
  label,
  isPending,
  onDelete,
}: {
  label: string;
  isPending: boolean;
  onDelete: () => Promise<void>;
}) {
  return (
    <button
      type="button"
      onClick={onDelete}
      disabled={isPending}
      className="inline-flex items-center gap-1.5 font-mono text-[10px] text-clay hover:opacity-80 disabled:opacity-50"
    >
      <Trash2 size={11} />
      {label}
    </button>
  );
}
