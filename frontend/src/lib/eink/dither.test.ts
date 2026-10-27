import { describe, expect, it } from 'bun:test';
import reference from './testdata/dither.json';
import { ditherTo1bpp, type DitherMode } from './dither';
import { autoContrast, autoInvert } from './preprocess';

// The fixture pins the dither output; the uploaded frame is exactly what the device shows,
// so algorithm changes must be deliberate.
describe('dither output matches the reference', () => {
  const gray = Uint8Array.from(reference.gray);
  const pre = autoContrast(autoInvert(gray, reference.width, reference.height), 1);

  it('preprocesses identically', () => {
    expect(Array.from(pre)).toEqual(reference.preprocessed);
  });

  for (const [key, hex] of Object.entries(reference.frames)) {
    it(key, () => {
      const [mode, threshold] = key.split(':');
      const out = ditherTo1bpp(pre, reference.width, reference.height, {
        mode: mode as DitherMode,
        threshold: Number(threshold),
      });
      expect(Buffer.from(out).toString('hex')).toBe(hex);
    });
  }
});
