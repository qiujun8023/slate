import { Prisma } from '@prisma/client';
import { NotFoundError } from '../errors';

/** SELECT … FOR UPDATE 行锁 users 表，按 owner 序列化 devices/groups 写入。 */
export async function lockUserRow(tx: Prisma.TransactionClient, userId: string): Promise<void> {
  const rows = await tx.$queryRaw<Array<{ id: string }>>`
    SELECT id FROM "users" WHERE id = ${userId} FOR UPDATE
  `;
  if (rows.length === 0) throw new NotFoundError('用户不存在');
}

/** SELECT … FOR UPDATE 行锁 groups 表，按 group 序列化 contents 写入。 */
export async function lockGroupRow(tx: Prisma.TransactionClient, groupId: string): Promise<void> {
  const rows = await tx.$queryRaw<Array<{ id: string }>>`
    SELECT id FROM "groups" WHERE id = ${groupId} FOR UPDATE
  `;
  if (rows.length === 0) throw new NotFoundError('相册不存在');
}
