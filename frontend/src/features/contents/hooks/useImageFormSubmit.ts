import { useCallback } from 'react';
import type { DitherMode } from '@/lib/eink/dither';
import { renderFrame, type CropOptions } from '@/lib/eink/image-preview';

export function useImageFormSubmit({
  imageFile,
  audioFile,
  frameName,
  threshold,
  mode,
  scale,
  offset,
}: {
  imageFile: File | null;
  audioFile: File | null;
  frameName: string;
  threshold: number;
  mode: DitherMode;
} & CropOptions) {
  return useCallback(async (): Promise<FormData> => {
    const fd = new FormData();
    if (imageFile) {
      const bitmap = await createImageBitmap(imageFile).catch(() => {
        throw new Error('图片无法解码，请换一张图片。');
      });
      try {
        const frame = renderFrame(bitmap, { scale, offset, threshold, mode });
        fd.append(
          'frame',
          new Blob([frame.slice()], { type: 'application/octet-stream' }),
          'frame.bin'
        );
      } finally {
        bitmap.close();
      }
    }
    if (audioFile) fd.append('audio', audioFile);
    fd.append('frameName', frameName.trim());
    return fd;
  }, [audioFile, frameName, imageFile, mode, offset, scale, threshold]);
}
