import type { Prisma } from '@prisma/client';
import type { PrismaService } from '../../infra/prisma/prisma.service';

export type PrismaClientLike = Prisma.TransactionClient | PrismaService;

type UniqueConstraint = { fields?: unknown; index?: unknown };

// driver adapter 下 P2002 不带 meta.target，冲突约束在 driverAdapterError.cause.constraint：
// PG 给出索引名（如 devices_mac_key），也可能直接给字段列表。
function uniqueTarget(err: Prisma.PrismaClientKnownRequestError): unknown {
  if (err.meta?.target !== undefined) return err.meta.target;
  const adapterError = err.meta?.driverAdapterError as
    { cause?: { constraint?: UniqueConstraint } } | undefined;
  const constraint = adapterError?.cause?.constraint;
  return constraint?.fields ?? constraint?.index;
}

export function prismaUniqueTargetIncludes(
  err: Prisma.PrismaClientKnownRequestError,
  ...fields: string[]
): boolean {
  if (err.code !== 'P2002') return false;
  const target = uniqueTarget(err);
  if (Array.isArray(target)) {
    return fields.every((field) => target.includes(field));
  }
  if (typeof target === 'string') {
    return fields.every((field) => target.includes(field));
  }
  return false;
}
