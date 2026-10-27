import { FrameHeight, FrameWidth } from '@/api';
import { INK_RGB, PAPER_HEX, PAPER_RGB } from './colors';

export function clearCanvas(
  ctx: CanvasRenderingContext2D,
  canvas?: { width: number; height: number }
): void {
  ctx.fillStyle = PAPER_HEX;
  ctx.fillRect(0, 0, canvas?.width ?? FrameWidth, canvas?.height ?? FrameHeight);
}

export function decodeBppImage(
  bytes: Uint8Array | ArrayBuffer,
  width: number = FrameWidth,
  height: number = FrameHeight,
  paperColor: readonly [number, number, number] = PAPER_RGB,
  inkColor: readonly [number, number, number] = INK_RGB
): ImageData {
  const byteView = bytes instanceof ArrayBuffer ? new Uint8Array(bytes) : bytes;
  const data = new ImageData(width, height);
  const bpr = width >> 3;
  const pixels = new Uint32Array(data.data.buffer);
  const paperPixel = rgbaPixel(paperColor);
  const inkPixel = rgbaPixel(inkColor);
  let dst = 0;

  for (let y = 0; y < height; y++) {
    const rowStart = y * bpr;
    for (let byteOffset = 0; byteOffset < bpr; byteOffset++) {
      const byte = byteView[rowStart + byteOffset]!;
      pixels[dst++] = byte & 0b1000_0000 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0100_0000 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0010_0000 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0001_0000 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0000_1000 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0000_0100 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0000_0010 ? paperPixel : inkPixel;
      pixels[dst++] = byte & 0b0000_0001 ? paperPixel : inkPixel;
    }
  }

  return data;
}

export function isValidBppLength(
  bytes: Uint8Array | ArrayBuffer,
  width: number = FrameWidth,
  height: number = FrameHeight
): boolean {
  const byteLength = bytes instanceof ArrayBuffer ? bytes.byteLength : bytes.length;
  return byteLength === (width * height) / 8;
}

const LITTLE_ENDIAN = new Uint8Array(new Uint32Array([0x0a0b0c0d]).buffer)[0] === 0x0d;

function rgbaPixel(color: readonly [number, number, number]): number {
  const [r, g, b] = color;
  return LITTLE_ENDIAN
    ? 0xff000000 | (b << 16) | (g << 8) | r
    : (r << 24) | (g << 16) | (b << 8) | 0xff;
}
