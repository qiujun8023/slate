import { z } from 'zod';
import { DEFAULT_TTS_VOICE, TtsVoice } from 'shared';

const emptyStringToUndefined = (value: unknown) =>
  typeof value === 'string' && value.trim() === '' ? undefined : value;

const OptionalEnv = (schema: z.ZodString) =>
  z.preprocess(emptyStringToUndefined, schema.optional());

const DatabaseUrl = z.string().refine(
  (value) => {
    try {
      return new URL(value).protocol === 'mysql:';
    } catch {
      return false;
    }
  },
  { message: 'DATABASE_URL must be a mysql:// connection string' }
);

const JwtSecret = z.string().min(32).refine(hasReasonableSecretEntropy, {
  message: 'JWT_SECRET must contain at least 16 distinct bytes and 128 bits of estimated entropy',
});

const JwtExpiration = z.string().refine(
  (value) => {
    const text = value.trim();
    if (/^\d+$/.test(text)) return true;
    return /^\d+(?:\.\d+)?\s*(?:ms|s|m|h|d|w|y)$/i.test(text);
  },
  { message: 'JWT_EXPIRATION must be a number of seconds or a duration like 15m, 7d, or 1h' }
);

const BooleanEnv = z.union([z.boolean(), z.string()]).transform((value, ctx) => {
  if (typeof value === 'boolean') return value;
  const normalized = value.trim().toLowerCase();
  if (['true', '1', 'yes', 'y', 'on'].includes(normalized)) return true;
  if (['false', '0', 'no', 'n', 'off'].includes(normalized)) return false;
  ctx.addIssue({ code: 'custom', message: 'invalid boolean value' });
  return z.NEVER;
});

export const EnvSchema = z.object({
  NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),
  LOG_LEVEL: z.enum(['debug', 'info', 'warn', 'error']).default('info'),
  PORT: z.coerce.number().int().positive().default(9494),
  DATABASE_URL: DatabaseUrl,
  JWT_SECRET: JwtSecret,
  JWT_EXPIRATION: JwtExpiration.default('7d'),
  BLOB_DIR: z.string().default('./blobs'),
  DB_ALLOW_PUBLIC_KEY_RETRIEVAL: BooleanEnv.default(true),
  QWEATHER_API_KEY: OptionalEnv(z.string().min(1)),
  QWEATHER_API_HOST: OptionalEnv(z.string().url()),
  AI_API_KEY: OptionalEnv(z.string().min(1)),
  AI_BASE_URL: OptionalEnv(z.string().url()),
  AI_MODEL: z.string().min(1).default('gpt-4o-mini'),
  TTS_API_KEY: OptionalEnv(z.string().min(1)),
  TTS_BASE_URL: OptionalEnv(z.string().url()),
  TTS_MODEL: z.string().min(1).default('mimo-v2.5-tts'),
  TTS_DEFAULT_VOICE: TtsVoice.default(DEFAULT_TTS_VOICE),
  BACKGROUND_WORKERS: BooleanEnv.default(true),
});

export type EnvT = z.infer<typeof EnvSchema>;

function hasReasonableSecretEntropy(value: string): boolean {
  const bytes = new TextEncoder().encode(value);
  if (new Set(bytes).size < 16) return false;
  return estimatedShannonBits(bytes) >= 128;
}

function estimatedShannonBits(bytes: Uint8Array): number {
  const counts = new Map<number, number>();
  for (const byte of bytes) counts.set(byte, (counts.get(byte) ?? 0) + 1);
  let entropyPerByte = 0;
  for (const count of counts.values()) {
    const p = count / bytes.length;
    entropyPerByte -= p * Math.log2(p);
  }
  return entropyPerByte * bytes.length;
}
