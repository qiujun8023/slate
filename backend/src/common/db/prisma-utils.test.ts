import { describe, expect, it } from 'bun:test';
import { Prisma } from '@prisma/client';
import { prismaUniqueTargetIncludes } from './prisma-utils';

function p2002(meta: Record<string, unknown>) {
  return new Prisma.PrismaClientKnownRequestError('Unique constraint failed', {
    code: 'P2002',
    clientVersion: 'test',
    meta,
  });
}

describe('prismaUniqueTargetIncludes', () => {
  it('reads meta.target field lists', () => {
    expect(prismaUniqueTargetIncludes(p2002({ target: ['pair_code'] }), 'pair_code')).toBe(true);
    expect(prismaUniqueTargetIncludes(p2002({ target: ['pair_code'] }), 'mac')).toBe(false);
  });

  it('reads the constraint index name reported by the pg driver adapter', () => {
    const err = p2002({
      driverAdapterError: {
        name: 'DriverAdapterError',
        cause: {
          originalCode: '23505',
          kind: 'UniqueConstraintViolation',
          constraint: { index: 'devices_mac_key' },
          table: 'devices',
        },
      },
      modelName: 'Device',
    });
    expect(prismaUniqueTargetIncludes(err, 'mac')).toBe(true);
    expect(prismaUniqueTargetIncludes(err, 'pair_code')).toBe(false);
  });

  it('reads constraint field lists reported by a driver adapter', () => {
    const err = p2002({
      driverAdapterError: { cause: { constraint: { fields: ['email'] } } },
    });
    expect(prismaUniqueTargetIncludes(err, 'email')).toBe(true);
  });

  it('returns false when no constraint information is available', () => {
    expect(prismaUniqueTargetIncludes(p2002({}), 'email')).toBe(false);
  });
});
