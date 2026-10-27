// dither 前的灰度预处理，顺序见 image-preview.ts：
//   1. 浏览器解码并绘制到 canvas，取 ImageData（RGBA）
//   2. rgbaToGray  — Rec.709 系数
//   3. autoInvert  — 四角自适应反相,黑底图整体翻成白底
//   4. autoContrast — PIL.ImageOps.autocontrast 等价,拉满对比度
//   5. ditherTo* — 见 dither.ts

/**
 * RGBA → 8-bit 灰度。
 * Rec.709 系数 (0.2126 R + 0.7152 G + 0.0722 B)。
 *
 * 接受 RGBA 或 RGB:通过 channels 参数区分(默认 4 = RGBA)。
 */
export function rgbaToGray(
  rgba: Uint8Array | Uint8ClampedArray,
  width: number,
  height: number,
  channels: 3 | 4 = 4
): Uint8Array {
  const expected = width * height * channels;
  if (rgba.length !== expected) {
    throw new Error(`rgba size mismatch: ${rgba.length} vs ${expected}`);
  }
  const out = new Uint8Array(width * height);
  for (let i = 0, j = 0; i < rgba.length; i += channels, j++) {
    const r = rgba[i]!;
    const g = rgba[i + 1]!;
    const b = rgba[i + 2]!;
    // 整数近似:0.2126 ≈ 54/256, 0.7152 ≈ 183/256, 0.0722 ≈ 19/256(和=256)
    out[j] = (r * 54 + g * 183 + b * 19) >> 8;
  }
  return out;
}

/**
 * 四角自适应反相:四个角内一格的均值若 < 128,视为黑底图,整体反相。
 * 返回新 buffer(不破坏输入)。
 */
export function autoInvert(gray: Uint8Array, width: number, height: number): Uint8Array {
  if (gray.length !== width * height) {
    throw new Error(`gray size mismatch: ${gray.length} vs ${width * height}`);
  }
  if (width < 3 || height < 3) {
    return copyOf(gray); // 太小不判断
  }
  const corners = [
    gray[1 * width + 1]!,
    gray[1 * width + (width - 2)]!,
    gray[(height - 2) * width + 1]!,
    gray[(height - 2) * width + (width - 2)]!,
  ];
  const mean = (corners[0]! + corners[1]! + corners[2]! + corners[3]!) / 4;
  if (mean >= 128) return copyOf(gray);

  const out = new Uint8Array(gray.length);
  for (let i = 0; i < gray.length; i++) out[i] = 255 - gray[i]!;
  return out;
}

/**
 * autocontrast:统计直方图,去掉两端 cutoffPct% 的离群像素后线性拉伸到 0..255。
 * 等价于 PIL.ImageOps.autocontrast(cutoff=cutoffPct)。
 */
export function autoContrast(gray: Uint8Array, cutoffPct: number): Uint8Array {
  const total = gray.length;
  const hist = new Int32Array(256);
  for (let i = 0; i < total; i++) hist[gray[i]!]!++;

  const cutoff = Math.floor((total * cutoffPct) / 100);
  let lo = 0;
  let hi = 255;
  let acc = 0;
  for (let i = 0; i < 256; i++) {
    acc += hist[i]!;
    if (acc > cutoff) {
      lo = i;
      break;
    }
  }
  acc = 0;
  for (let i = 255; i >= 0; i--) {
    acc += hist[i]!;
    if (acc > cutoff) {
      hi = i;
      break;
    }
  }
  if (hi <= lo) return copyOf(gray); // 单色或退化:不变

  const scale = 255 / (hi - lo);
  const out = new Uint8Array(total);
  for (let i = 0; i < total; i++) {
    const v = gray[i]!;
    if (v <= lo) out[i] = 0;
    else if (v >= hi) out[i] = 255;
    else out[i] = Math.round((v - lo) * scale);
  }
  return out;
}

function copyOf(src: Uint8Array): Uint8Array {
  const out = new Uint8Array(src.length);
  out.set(src);
  return out;
}
