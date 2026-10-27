// 1bpp dither。预览和上传的设备帧都由这里生成，输出由 dither.test.ts 的 fixture 固定。
//
// 输入:8-bit 灰度 buffer(0=黑 255=白),无 alpha。
// 输出二选一:
//   - ditherTo1bpp:packed 1bpp,MSB-first,bit=1 白 / bit=0 黑。
//                  字节序和 firmware/main/drivers/display/epd_ssd1683.cc 的 Pack1bppTo2683 对齐。
//   - ditherToBinary:每像素 0 或 255 灰度,方便前端 putImageData 直接画。
//
// 6 种模式:
//   - threshold:硬阈值(线稿/简笔画用)
//   - bayer4 / bayer8:有序抖动(快;有规则点阵纹路)
//   - floyd:Floyd-Steinberg(照片首选,4 邻像素扩散)
//   - atkinson:Atkinson(老 Mac 风;只扩散 6/8 误差,高对比)
//   - sierra:Sierra Lite(柔和,3 邻像素)

export const DITHER_MODES = [
  { id: 'threshold', label: '线稿 · 纯黑白', hint: '硬阈值；无中间灰' },
  { id: 'bayer4', label: '网点 · 粗', hint: 'Bayer 4×4；复古海报感' },
  { id: 'bayer8', label: '网点 · 细', hint: 'Bayer 8×8；点阵更细' },
  { id: 'floyd', label: '照片 · 推荐', hint: 'Floyd-Steinberg；颗粒细腻' },
  { id: 'atkinson', label: '照片 · 高对比', hint: 'Atkinson；亮的更亮' },
  { id: 'sierra', label: '照片 · 柔和', hint: 'Sierra Lite；接近 FS 更轻' },
] as const;

export type DitherMode = (typeof DITHER_MODES)[number]['id'];

export const DEFAULT_DITHER_MODE: DitherMode = 'floyd';

export interface DitherOptions {
  mode: DitherMode;
  /** 0..255 切分中点。所有模式都生效 — 误差扩散类用它调整亮暗平衡。默认 128 */
  threshold?: number;
  /** 误差扩散用蛇形扫描,减少方向条纹。默认 true */
  serpentine?: boolean;
  /** dither 前 sRGB→linear、阈值同步映射。中调更准。默认 true */
  gammaCorrect?: boolean;
}

// ---------- LUT ----------

const SRGB_TO_LINEAR: Uint8Array = (() => {
  const t = new Uint8Array(256);
  for (let i = 0; i < 256; i++) {
    const s = i / 255;
    const lin = s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
    t[i] = Math.round(lin * 255);
  }
  return t;
})();

// 标准 Bresenham Bayer 矩阵
const BAYER_4: readonly number[] = [0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5];

const BAYER_8: readonly number[] = [
  0, 32, 8, 40, 2, 34, 10, 42, 48, 16, 56, 24, 50, 18, 58, 26, 12, 44, 4, 36, 14, 46, 6, 38, 60, 28,
  52, 20, 62, 30, 54, 22, 3, 35, 11, 43, 1, 33, 9, 41, 51, 19, 59, 27, 49, 17, 57, 25, 15, 47, 7,
  39, 13, 45, 5, 37, 63, 31, 55, 23, 61, 29, 53, 21,
];

interface DiffuseEntry {
  dx: number;
  dy: number;
  w: number;
}
interface DiffuseKernel {
  entries: readonly DiffuseEntry[];
  divisor: number;
}

const FLOYD_KERNEL: DiffuseKernel = {
  entries: [
    { dx: 1, dy: 0, w: 7 },
    { dx: -1, dy: 1, w: 3 },
    { dx: 0, dy: 1, w: 5 },
    { dx: 1, dy: 1, w: 1 },
  ],
  divisor: 16,
};

// Atkinson 总权重 6,divisor 8 — 故意少传 25% 误差,保留对比
const ATKINSON_KERNEL: DiffuseKernel = {
  entries: [
    { dx: 1, dy: 0, w: 1 },
    { dx: 2, dy: 0, w: 1 },
    { dx: -1, dy: 1, w: 1 },
    { dx: 0, dy: 1, w: 1 },
    { dx: 1, dy: 1, w: 1 },
    { dx: 0, dy: 2, w: 1 },
  ],
  divisor: 8,
};

const SIERRA_LITE_KERNEL: DiffuseKernel = {
  entries: [
    { dx: 1, dy: 0, w: 2 },
    { dx: -1, dy: 1, w: 1 },
    { dx: 0, dy: 1, w: 1 },
  ],
  divisor: 4,
};

// ---------- public ----------

/** 灰度 → 二值灰度(每像素 0 或 255) */
export function ditherToBinary(
  gray: Uint8Array | Uint8ClampedArray,
  width: number,
  height: number,
  options: DitherOptions
): Uint8Array {
  const bits = runDither(gray, width, height, normalizeOptions(options));
  const out = new Uint8Array(bits.length);
  for (let i = 0; i < bits.length; i++) out[i] = bits[i] ? 255 : 0;
  return out;
}

/** 灰度 → 1bpp packed (MSB-first, bit=1 白 0 黑) */
export function ditherTo1bpp(
  gray: Uint8Array | Uint8ClampedArray,
  width: number,
  height: number,
  options: DitherOptions
): Uint8Array {
  if (width % 8 !== 0) {
    throw new Error(`width must be multiple of 8, got ${width}`);
  }
  const bits = runDither(gray, width, height, normalizeOptions(options));
  const out = new Uint8Array((width * height) / 8);
  out.fill(0xff);
  const bpr = width >> 3;
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      if (bits[y * width + x] === 0) {
        const byteIdx = y * bpr + (x >> 3);
        out[byteIdx]! &= ~(1 << (7 - (x & 7)));
      }
    }
  }
  return out;
}

// ---------- internals ----------

interface NormalizedOpts {
  mode: DitherMode;
  threshold: number;
  serpentine: boolean;
  gammaCorrect: boolean;
}

function normalizeOptions(o: DitherOptions): NormalizedOpts {
  return {
    mode: o.mode,
    threshold: o.threshold ?? 128,
    serpentine: o.serpentine ?? true,
    gammaCorrect: o.gammaCorrect ?? true,
  };
}

/**
 * 主 dispatcher。返回每像素 1 个 byte 的 Uint8Array,值 ∈ {0, 1}(0=黑 1=白)。
 *
 * gamma 处理:开启时把 gray 和 threshold 都 sRGB→linear 后再 dither。
 * 端点 0/255 不变,所以 dither 输出可直接当 sRGB 0/255 用,不必再回 sRGB。
 */
function runDither(
  gray: Uint8Array | Uint8ClampedArray,
  w: number,
  h: number,
  opts: NormalizedOpts
): Uint8Array {
  if (gray.length !== w * h) {
    throw new Error(`gray buffer size mismatch: ${gray.length} vs ${w * h}`);
  }

  const buf = opts.gammaCorrect ? mapLut(gray, SRGB_TO_LINEAR) : copyTo(gray);
  const t = opts.gammaCorrect ? SRGB_TO_LINEAR[opts.threshold]! : opts.threshold;
  const bits = new Uint8Array(w * h);

  switch (opts.mode) {
    case 'threshold':
      for (let i = 0; i < buf.length; i++) bits[i] = buf[i]! >= t ? 1 : 0;
      return bits;

    case 'bayer4':
      return bayerDither(buf, w, h, BAYER_4, 4, 16, t, bits);
    case 'bayer8':
      return bayerDither(buf, w, h, BAYER_8, 8, 64, t, bits);

    case 'floyd':
      return errorDiffuse(buf, w, h, FLOYD_KERNEL, t, opts.serpentine, bits);
    case 'atkinson':
      return errorDiffuse(buf, w, h, ATKINSON_KERNEL, t, opts.serpentine, bits);
    case 'sierra':
      return errorDiffuse(buf, w, h, SIERRA_LITE_KERNEL, t, opts.serpentine, bits);
  }
}

function copyTo(src: Uint8Array | Uint8ClampedArray): Uint8Array {
  const out = new Uint8Array(src.length);
  out.set(src);
  return out;
}

function mapLut(src: Uint8Array | Uint8ClampedArray, lut: Uint8Array): Uint8Array {
  const out = new Uint8Array(src.length);
  for (let i = 0; i < src.length; i++) out[i] = lut[src[i]!]!;
  return out;
}

function bayerDither(
  buf: Uint8Array,
  w: number,
  h: number,
  matrix: readonly number[],
  size: number,
  levels: number,
  threshold: number,
  out: Uint8Array
): Uint8Array {
  // 每像素加 [-span/2, +span/2) 的偏移再和 threshold 比较。
  // span=128 是经验值:更大点阵感强,更小接近硬阈值。
  const span = 128;
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const m = matrix[(y % size) * size + (x % size)]!;
      const offset = ((m + 0.5) / levels - 0.5) * span;
      const v = buf[y * w + x]! + offset;
      out[y * w + x] = v >= threshold ? 1 : 0;
    }
  }
  return out;
}

function errorDiffuse(
  buf: Uint8Array,
  w: number,
  h: number,
  kernel: DiffuseKernel,
  threshold: number,
  serpentine: boolean,
  out: Uint8Array
): Uint8Array {
  // 用 Int16 暂存,避免 clamp 损失误差精度
  const work = new Int16Array(w * h);
  for (let i = 0; i < buf.length; i++) work[i] = buf[i]!;

  for (let y = 0; y < h; y++) {
    const reverse = serpentine && (y & 1) === 1;
    const xStart = reverse ? w - 1 : 0;
    const xEnd = reverse ? -1 : w;
    const xStep = reverse ? -1 : 1;

    for (let x = xStart; x !== xEnd; x += xStep) {
      const idx = y * w + x;
      const old = work[idx]!;
      const isWhite = old >= threshold;
      out[idx] = isWhite ? 1 : 0;
      const newVal = isWhite ? 255 : 0;
      const err = old - newVal;
      if (err === 0) continue;

      for (const e of kernel.entries) {
        const dx = reverse ? -e.dx : e.dx;
        const nx = x + dx;
        const ny = y + e.dy;
        // 部分扩散核会向上一行传播误差，两个方向都需要边界保护。
        if (nx < 0 || nx >= w || ny < 0 || ny >= h) continue;
        const ni = ny * w + nx;
        work[ni] += Math.trunc((err * e.w) / kernel.divisor);
      }
    }
  }
  return out;
}
