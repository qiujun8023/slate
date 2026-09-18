import { describe, expect, it } from 'bun:test';
import { EnvSchema } from './env.schema';

const baseEnv = {
  DATABASE_URL: 'mysql://slate:slate@127.0.0.1:3306/slate',
  JWT_SECRET: 'slate-test-secret-0123456789abcdef',
};

describe('EnvSchema', () => {
  it('accepts mysql database connection strings', () => {
    expect(EnvSchema.safeParse(baseEnv).success).toBe(true);
  });

  it('rejects non-database URLs for DATABASE_URL', () => {
    expect(EnvSchema.safeParse({ ...baseEnv, DATABASE_URL: 'https://example.com' }).success).toBe(
      false
    );
  });

  it('rejects low-entropy JWT secrets', () => {
    expect(EnvSchema.safeParse({ ...baseEnv, JWT_SECRET: 'x'.repeat(32) }).success).toBe(false);
  });

  it('rejects invalid JWT expiration values at startup validation time', () => {
    expect(EnvSchema.safeParse({ ...baseEnv, JWT_EXPIRATION: 'abc' }).success).toBe(false);
  });

  it('rejects default TTS voices outside the shared voice catalog', () => {
    expect(EnvSchema.safeParse({ ...baseEnv, TTS_DEFAULT_VOICE: '不存在的音色' }).success).toBe(
      false
    );
  });

  it('defaults public key retrieval to enabled', () => {
    const parsed = EnvSchema.parse(baseEnv);

    expect(parsed.DB_ALLOW_PUBLIC_KEY_RETRIEVAL).toBe(true);
  });
});
