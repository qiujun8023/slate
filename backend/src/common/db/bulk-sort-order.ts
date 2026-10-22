import { Prisma } from '@prisma/client';
import { ValidationError } from '../errors';

const ALLOWED_TABLES = new Set(['devices', 'groups', 'contents']);
const ALLOWED_SCOPE_COLUMNS = new Set(['owner_user_id', 'group_id']);

export function assertSortOrderIdentifiers(table: string, scopeColumn: string): void {
  if (!ALLOWED_TABLES.has(table)) {
    throw new ValidationError('非法排序表名', { code: 'invalid_sort_table' });
  }
  if (!ALLOWED_SCOPE_COLUMNS.has(scopeColumn)) {
    throw new ValidationError('非法排序作用域列', { code: 'invalid_sort_scope_column' });
  }
}

// 把 order 中的 id 按下标依次写回该表的 sort_order 列。
// 两步式 staging：先把每行写到唯一的负数槽位，再 finalize 成 0..N-1，避开 (scope, sort_order)
// 唯一约束在 reshuffle 中出现的中间冲突。
async function bulkSetSortOrder(
  tx: Prisma.TransactionClient,
  table: 'devices' | 'groups' | 'contents',
  scopeColumn: 'owner_user_id' | 'group_id',
  scopeValue: string,
  order: string[]
): Promise<void> {
  if (order.length === 0) return;
  if (new Set(order).size !== order.length) {
    throw new ValidationError('排序列表不能包含重复 ID', { code: 'order_duplicate' });
  }
  assertSortOrderIdentifiers(table, scopeColumn);
  const tableSql = Prisma.raw(`"${table}"`);
  const scopeColSql = Prisma.raw(`"${scopeColumn}"`);
  const ids = Prisma.join(order);
  for (const stage of ['staging', 'final'] as const) {
    await tx.$executeRaw`
      UPDATE ${tableSql}
      SET "sort_order" = CASE "id"
        ${Prisma.join(
          order.map(
            (id, idx) => Prisma.sql`WHEN ${id} THEN ${stage === 'final' ? idx : -(idx + 1)}::int`
          ),
          ' '
        )}
      END
      WHERE ${scopeColSql} = ${scopeValue} AND "id" IN (${ids})
    `;
  }
}

export function bulkSetDeviceSortOrder(
  tx: Prisma.TransactionClient,
  ownerUserId: string,
  order: string[]
): Promise<void> {
  return bulkSetSortOrder(tx, 'devices', 'owner_user_id', ownerUserId, order);
}

export function bulkSetGroupSortOrder(
  tx: Prisma.TransactionClient,
  ownerUserId: string,
  order: string[]
): Promise<void> {
  return bulkSetSortOrder(tx, 'groups', 'owner_user_id', ownerUserId, order);
}

export function bulkSetContentSortOrder(
  tx: Prisma.TransactionClient,
  groupId: string,
  order: string[]
): Promise<void> {
  return bulkSetSortOrder(tx, 'contents', 'group_id', groupId, order);
}

export async function compactContentSortOrders(
  tx: Prisma.TransactionClient,
  groupId: string
): Promise<void> {
  const rows = await tx.content.findMany({
    where: { groupId },
    orderBy: [{ sortOrder: 'asc' }, { createdAt: 'asc' }],
    select: { id: true },
  });
  await bulkSetContentSortOrder(
    tx,
    groupId,
    rows.map((row) => row.id)
  );
}
