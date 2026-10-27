import { FrameHeight, FrameWidth } from '@/api';
import { ditherTo1bpp, ditherToBinary, type DitherMode } from './dither';
import { autoContrast, autoInvert, rgbaToGray } from './preprocess';
import { clearCanvas } from './bpp';
import { INK_RGB, PAPER_RGB } from './colors';

export interface CropOptions {
  scale: number;
  offset: { x: number; y: number };
}

export interface DitherSettings {
  threshold: number;
  mode: DitherMode;
}

type Source = HTMLImageElement | ImageBitmap;

/** Draws the crop onto the preview canvas, dithered to ink and paper when `dither` is set. */
export function drawImagePreview(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  img: Source,
  { dither, ...options }: CropOptions & DitherSettings & { dither: boolean }
) {
  drawCrop(ctx, canvas, img, options);
  if (!dither) return;

  const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const bin = ditherToBinary(preprocess(imageData, canvas), canvas.width, canvas.height, options);
  for (let i = 0, j = 0; i < imageData.data.length; i += 4, j++) {
    const color = bin[j] === 255 ? PAPER_RGB : INK_RGB;
    imageData.data[i] = color[0];
    imageData.data[i + 1] = color[1];
    imageData.data[i + 2] = color[2];
    imageData.data[i + 3] = 255;
  }
  ctx.putImageData(imageData, 0, 0);
}

/**
 * Renders the device frame the preview shows: 400×300 packed 1bpp, MSB first, bit 1 = white.
 * This is what gets uploaded; the server stores it as is.
 */
export function renderFrame(img: Source, options: CropOptions & DitherSettings): Uint8Array {
  const canvas = document.createElement('canvas');
  canvas.width = FrameWidth;
  canvas.height = FrameHeight;
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) throw new Error('canvas 2d context unavailable');
  drawCrop(ctx, canvas, img, options);
  const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
  return ditherTo1bpp(preprocess(imageData, canvas), canvas.width, canvas.height, options);
}

function drawCrop(
  ctx: CanvasRenderingContext2D,
  canvas: { width: number; height: number },
  img: Source,
  { scale, offset }: CropOptions
) {
  clearCanvas(ctx, canvas);
  const sourceWidth = img instanceof HTMLImageElement ? img.naturalWidth || img.width : img.width;
  const sourceHeight =
    img instanceof HTMLImageElement ? img.naturalHeight || img.height : img.height;
  const finalScale = Math.min(canvas.width / sourceWidth, canvas.height / sourceHeight) * scale;
  const drawW = sourceWidth * finalScale;
  const drawH = sourceHeight * finalScale;
  const drawX = (canvas.width - drawW) / 2 + offset.x;
  const drawY = (canvas.height - drawH) / 2 + offset.y;
  ctx.drawImage(img, drawX, drawY, drawW, drawH);
}

function preprocess(imageData: ImageData, canvas: { width: number; height: number }): Uint8Array {
  const gray = autoInvert(
    rgbaToGray(imageData.data, canvas.width, canvas.height, 4),
    canvas.width,
    canvas.height
  );
  return autoContrast(gray, 1);
}
