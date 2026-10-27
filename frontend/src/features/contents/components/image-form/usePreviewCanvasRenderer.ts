import { useCallback, useEffect, useRef, type RefObject } from 'react';
import type { DitherMode } from '@/lib/eink/dither';
import { clearCanvas, decodeBppImage, isValidBppLength } from '@/lib/eink/bpp';
import { drawImagePreview } from '@/lib/eink/image-preview';
import { useCanvasPan } from './useCanvasPan';
import { useLoadedImage } from './useLoadedImage';

interface CanvasOffset {
  x: number;
  y: number;
}

export function usePreviewCanvasRenderer({
  imageFile,
  existingImage,
  threshold,
  mode,
  scale,
  offset,
  onOffsetChange,
  canvasRef,
}: {
  imageFile: File | null;
  existingImage: ArrayBuffer | undefined;
  threshold: number;
  mode: DitherMode;
  scale: number;
  offset: CanvasOffset;
  onOffsetChange: (offset: CanvasOffset) => void;
  canvasRef: RefObject<HTMLCanvasElement | null>;
}) {
  const loadedImage = useLoadedImage(imageFile);
  const drawFrameRef = useRef<number | null>(null);
  const currentOffsetRef = useRef(offset);

  const cancelScheduledDraw = useCallback(() => {
    if (drawFrameRef.current == null) return;
    cancelAnimationFrame(drawFrameRef.current);
    drawFrameRef.current = null;
  }, []);

  useEffect(() => cancelScheduledDraw, [cancelScheduledDraw]);

  const drawLoadedImage = useCallback(
    (dither: boolean) => {
      if (!imageFile || loadedImage?.file !== imageFile) return false;
      const canvas = canvasRef.current;
      if (!canvas) return false;
      const ctx = canvas.getContext('2d');
      if (!ctx) return false;
      drawImagePreview(ctx, canvas, loadedImage.image, {
        scale,
        offset: currentOffsetRef.current,
        threshold,
        mode,
        dither,
      });
      return true;
    },
    [canvasRef, imageFile, loadedImage, mode, scale, threshold]
  );

  const scheduleImageDraw = useCallback(
    (dither: boolean) => {
      cancelScheduledDraw();
      drawFrameRef.current = requestAnimationFrame(() => {
        drawFrameRef.current = null;
        drawLoadedImage(dither);
      });
    },
    [cancelScheduledDraw, drawLoadedImage]
  );

  const pan = useCanvasPan({
    enabled: !!imageFile && loadedImage?.file === imageFile,
    offset,
    currentOffsetRef,
    onOffsetChange,
    onPreviewMove: () => scheduleImageDraw(false),
    onCommit: () => scheduleImageDraw(true),
  });
  const { isDraggingRef } = pan;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    if (imageFile) {
      if (loadedImage?.file !== imageFile) {
        cancelScheduledDraw();
        clearCanvas(ctx, canvas);
        return;
      }
      if (!isDraggingRef.current) {
        currentOffsetRef.current = offset;
      }
      scheduleImageDraw(!isDraggingRef.current);
      return cancelScheduledDraw;
    }

    cancelScheduledDraw();
    if (existingImage) {
      const bytes = new Uint8Array(existingImage);
      if (!isValidBppLength(bytes)) {
        clearCanvas(ctx, canvas);
        return;
      }
      const data = decodeBppImage(bytes);
      ctx.putImageData(data, 0, 0);
      return;
    }

    clearCanvas(ctx, canvas);
  }, [
    canvasRef,
    cancelScheduledDraw,
    existingImage,
    imageFile,
    isDraggingRef,
    loadedImage,
    offset,
    scheduleImageDraw,
  ]);

  return pan;
}
