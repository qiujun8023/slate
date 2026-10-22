import { describe, expect, it } from 'bun:test';
import { ValidationError } from '../errors';
import {
  assertSortOrderIdentifiers,
  bulkSetDeviceSortOrder,
  bulkSetGroupSortOrder,
} from './bulk-sort-order';

describe('bulk sort order helpers', () => {
  it('rejects duplicate ids before generating CASE updates', async () => {
    let executeCalls = 0;
    const tx = {
      $executeRaw: async () => {
        executeCalls += 1;
      },
    };

    await expect(
      bulkSetGroupSortOrder(tx as never, 'user-1', ['group-1', 'group-1'])
    ).rejects.toThrow(ValidationError);
    expect(executeCalls).toBe(0);
  });

  it('validates SQL identifiers at runtime before using Prisma.raw', async () => {
    let executeCalls = 0;
    const tx = {
      $executeRaw: async () => {
        executeCalls += 1;
      },
    };

    await expect(
      bulkSetDeviceSortOrder(tx as never, 'user-1', ['device-1'])
    ).resolves.toBeUndefined();
    expect(() =>
      assertSortOrderIdentifiers('groups" SET sort_order = 0 --', 'owner_user_id')
    ).toThrow(ValidationError);
    expect(() => assertSortOrderIdentifiers('groups', 'owner_user_id` = owner_user_id --')).toThrow(
      ValidationError
    );
    expect(executeCalls).toBe(2);
  });
});
