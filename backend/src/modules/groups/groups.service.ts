import { Injectable, Logger } from '@nestjs/common';
import type { GroupSummaryT } from 'shared';
import { Prisma } from '@prisma/client';
import { PrismaService } from '../../infra/prisma/prisma.service';
import { BlobService } from '../../infra/blob/blob.service';
import { ForbiddenError, NotFoundError } from '../../common/errors';
import { lockUserRow } from '../../common/db/row-locks';
import { bulkSetGroupSortOrder } from '../../common/db/bulk-sort-order';
import { validateOrderSet } from '../../common/db/order-validation';
import { nextGroupSortOrder } from '../../common/db/sort-order';
import type { PrismaClientLike } from '../../common/db/prisma-utils';
import { audioBlobContentId } from '../../infra/blob/content-audio-blobs';
import { computeGroupEtags, computeGroupManifestEtag } from './group-etag';

interface GroupListEntry {
  id: string;
  name: string;
  structureEtag: string;
  manifestEtag: string;
  sortOrder: number;
  _count: { contents: number };
}

export interface CycleResult {
  groupId: string | null;
  name: string | null;
  structureEtag: string | null;
  manifestEtag: string | null;
  sortOrder: number | null;
  contentCount: number;
  position: { current: number; total: number } | null;
}

export interface DeviceGroupSnapshot {
  ownerUserId: string | null;
  selectedGroupId: string | null;
}

export interface GroupEtags {
  structureEtag: string;
  manifestEtag: string;
  contentEtags: Array<{ id: string; etag: string; previousEtag: string }>;
}

@Injectable()
export class GroupsService {
  private readonly logger = new Logger(GroupsService.name);

  constructor(
    private readonly prisma: PrismaService,
    private readonly blob: BlobService
  ) {}

  // ── etag ──────────────────────────────────────────────────

  /** content_etag 覆盖单帧摘要；structure_etag 只覆盖顺序/增删；manifest_etag 覆盖完整 manifest。 */
  async recomputeManifestEtag(groupId: string, client?: PrismaClientLike): Promise<string> {
    const etags = await this.recomputeGroupEtags(groupId, client);
    return etags.manifestEtag;
  }

  async recomputeGroupEtags(groupId: string, client?: PrismaClientLike): Promise<GroupEtags> {
    if (!client) {
      return this.prisma.$transaction((tx) => this.recomputeGroupEtags(groupId, tx));
    }

    const group = await client.group.findUnique({
      where: { id: groupId },
      include: {
        contents: {
          orderBy: { sortOrder: 'asc' },
          select: {
            id: true,
            sortOrder: true,
            kind: true,
            dynamicType: true,
            imageEtag: true,
            imageSize: true,
            audioEtag: true,
            audioSize: true,
            audioStatus: true,
            audioSource: true,
            audioVoice: true,
            frameName: true,
            dynamicConfig: true,
            dynamicData: true,
            dynamicLastRunAt: true,
            contentEtag: true,
          },
        },
      },
    });
    if (!group) throw new NotFoundError(`相册 ${groupId} 不存在`);

    const { structureEtag, manifestEtag, contentEtags } = computeGroupEtags(group);

    await bulkSetContentEtags(
      client,
      contentEtags.filter((content) => content.etag !== content.previousEtag)
    );
    await client.group.update({
      where: { id: groupId },
      data: { structureEtag, manifestEtag },
    });
    return { structureEtag, manifestEtag, contentEtags };
  }

  // ── 设备 cycle / select / describe ─────────────────────────

  async listOwnerGroups(
    ownerUserId: string,
    client: PrismaClientLike = this.prisma
  ): Promise<GroupListEntry[]> {
    return this.queryOwnerGroups(ownerUserId, client);
  }

  async setDeviceGroup(deviceId: string, gid: string): Promise<void> {
    await this.prisma.$transaction(async (tx) => {
      const device = await tx.device.findUnique({
        where: { id: deviceId },
        select: { ownerUserId: true },
      });
      if (!device) throw new NotFoundError(`设备 ${deviceId} 不存在`);
      if (!device.ownerUserId) {
        throw new ForbiddenError('未绑定设备无法分配相册', {
          code: 'device_not_owned',
        });
      }
      await lockUserRow(tx, device.ownerUserId);

      const group = await tx.group.findUnique({
        where: { id: gid },
        select: { ownerUserId: true },
      });
      if (!group || group.ownerUserId !== device.ownerUserId) {
        throw new ForbiddenError('所选相册不属于该设备的拥有者', {
          code: 'group_not_in_scope',
        });
      }

      await tx.device.update({
        where: { id: deviceId },
        data: { selectedGroupId: gid },
      });
    });
  }

  async cycleDeviceGroup(deviceId: string, direction: 'next' | 'prev'): Promise<CycleResult> {
    return this.prisma.$transaction(async (tx) => {
      const device = await tx.device.findUnique({
        where: { id: deviceId },
        select: { ownerUserId: true, selectedGroupId: true },
      });
      if (!device) throw new NotFoundError(`设备 ${deviceId} 不存在`);
      if (!device.ownerUserId) return emptyCycle();
      await lockUserRow(tx, device.ownerUserId);

      const groups = await this.listOwnerGroups(device.ownerUserId, tx);
      if (groups.length === 0) return emptyCycle();

      const curIdx = device.selectedGroupId
        ? groups.findIndex((g) => g.id === device.selectedGroupId)
        : -1;
      const nextIdx =
        curIdx < 0
          ? direction === 'next'
            ? 0
            : groups.length - 1
          : (curIdx + (direction === 'next' ? 1 : -1) + groups.length) % groups.length;
      const target = groups[nextIdx]!;

      await tx.device.update({
        where: { id: deviceId },
        data: { selectedGroupId: target.id },
      });

      return {
        groupId: target.id,
        name: target.name,
        structureEtag: target.structureEtag,
        manifestEtag: target.manifestEtag,
        sortOrder: target.sortOrder,
        contentCount: target._count.contents,
        position: { current: nextIdx + 1, total: groups.length },
      };
    });
  }

  async describeDeviceGroup(deviceId: string): Promise<CycleResult> {
    const device = await this.prisma.device.findUnique({
      where: { id: deviceId },
      select: { ownerUserId: true, selectedGroupId: true },
    });
    if (!device) throw new NotFoundError(`设备 ${deviceId} 不存在`);
    return this.describeDeviceGroupSnapshot(device);
  }

  async describeDeviceGroupSnapshot(device: DeviceGroupSnapshot): Promise<CycleResult> {
    if (!device.ownerUserId || !device.selectedGroupId) return emptyCycle();
    const g = await this.prisma.group.findFirst({
      where: { id: device.selectedGroupId, ownerUserId: device.ownerUserId },
      select: {
        id: true,
        name: true,
        structureEtag: true,
        manifestEtag: true,
        sortOrder: true,
        _count: { select: { contents: true } },
      },
    });
    if (!g) return emptyCycle();

    const position = await this.ownerGroupPosition(device.ownerUserId, g.sortOrder);
    return {
      groupId: g.id,
      name: g.name,
      structureEtag: g.structureEtag,
      manifestEtag: g.manifestEtag,
      sortOrder: g.sortOrder,
      contentCount: g._count.contents,
      position,
    };
  }

  async ownerGroupPosition(
    ownerUserId: string,
    sortOrder: number,
    client: PrismaClientLike = this.prisma
  ): Promise<{ current: number; total: number }> {
    const [beforeCount, totalCount] = await Promise.all([
      client.group.count({
        where: { ownerUserId, sortOrder: { lt: sortOrder } },
      }),
      client.group.count({ where: { ownerUserId } }),
    ]);
    return { current: beforeCount + 1, total: Math.max(totalCount, 1) };
  }

  // ── Web CRUD ──────────────────────────────────────────────

  async listForOwner(ownerUserId: string): Promise<GroupSummaryT[]> {
    const [groups, sizeMap] = await Promise.all([
      this.queryOwnerGroups(ownerUserId),
      this.aggregateBytesForOwner(ownerUserId),
    ]);
    return groups.map((g) => toSummary(g, sizeMap.get(g.id) ?? 0));
  }

  async getOwned(gid: string, ownerUserId: string): Promise<GroupSummaryT> {
    const g = await this.prisma.group.findUnique({
      where: { id: gid },
      select: {
        id: true,
        name: true,
        ownerUserId: true,
        structureEtag: true,
        manifestEtag: true,
        sortOrder: true,
        _count: { select: { contents: true } },
      },
    });
    if (!g || g.ownerUserId !== ownerUserId) {
      throw new NotFoundError('相册不存在');
    }
    const sizeMap = await this.aggregateBytes([gid]);
    return toSummary(g, sizeMap.get(gid) ?? 0);
  }

  async create(ownerUserId: string, body: { name: string }): Promise<GroupSummaryT> {
    const created = await this.prisma.$transaction(async (tx) => {
      await lockUserRow(tx, ownerUserId);
      const sortOrder = await nextGroupSortOrder(tx, ownerUserId);
      const group = await tx.group.create({
        data: {
          name: body.name,
          structureEtag: 'empty',
          manifestEtag: 'empty',
          ownerUserId,
          sortOrder,
        },
        include: { _count: { select: { contents: true } } },
      });

      // 反向自动绑定：这是 owner 的第一个相册时，把所有 selectedGroupId=null 的已绑设备都指过来。
      // 配合 claim 时的「已有相册则自动绑第一个」，新用户全程不需要再去设备详情手动「分配相册」。
      // 仅在 count==1 时触发，避免后续创建相册时覆盖用户主动留空的设备。
      const groupCount = await tx.group.count({ where: { ownerUserId } });
      if (groupCount === 1) {
        const result = await tx.device.updateMany({
          where: { ownerUserId, selectedGroupId: null },
          data: { selectedGroupId: group.id },
        });
        if (result.count > 0) {
          this.logger.log(
            `First group ${group.id} was created for owner ${ownerUserId}; ${result.count} pending device(s) were auto-bound.`
          );
        }
      }
      return group;
    });
    return toSummary(created, 0);
  }

  async update(gid: string, ownerUserId: string, body: { name?: string }): Promise<GroupSummaryT> {
    // 校验 + 更新 + recomputeManifestEtag 收进同一事务；name 没变直接跳过 update。
    const group = await this.prisma.$transaction(async (tx) => {
      await lockUserRow(tx, ownerUserId);
      const g = await tx.group.findUnique({
        where: { id: gid },
        include: { _count: { select: { contents: true } } },
      });
      if (!g || g.ownerUserId !== ownerUserId) {
        throw new NotFoundError('相册不存在');
      }
      if (body.name === undefined || body.name === g.name) return g;
      await tx.group.update({ where: { id: gid }, data: { name: body.name } });
      const etags = await this.recomputeGroupEtags(gid, tx);
      return { ...g, name: body.name, ...etags };
    });
    const sizeMap = await this.aggregateBytes([gid]);
    return toSummary(group, sizeMap.get(gid) ?? 0);
  }

  async delete(gid: string, ownerUserId: string): Promise<void> {
    const g = await this.prisma.$transaction(async (tx) => {
      await lockUserRow(tx, ownerUserId);
      const group = await tx.group.findUnique({
        where: { id: gid },
        include: {
          contents: { select: { id: true, audioEtag: true } },
        },
      });
      if (!group || group.ownerUserId !== ownerUserId) {
        throw new NotFoundError('相册不存在');
      }
      await tx.group.delete({ where: { id: gid } });
      return group;
    });
    const deleted = await Promise.allSettled(
      g.contents.flatMap((content) => {
        return [
          this.blob.delete(gid, content.id, 'image'),
          content.audioEtag
            ? this.blob.delete(gid, audioBlobContentId(content.id, content.audioEtag), 'audio')
            : Promise.resolve(),
        ];
      })
    );
    const failed = deleted.filter((result) => result.status === 'rejected').length;
    if (failed > 0) {
      this.logger.warn(`Group ${gid} was deleted, but ${failed} blob cleanup operation(s) failed.`);
    }
  }

  async reorderGroups(ownerUserId: string, order: string[]): Promise<void> {
    await this.prisma.$transaction(async (tx) => {
      await lockUserRow(tx, ownerUserId);
      const owned = await tx.group.findMany({
        where: { ownerUserId },
        select: { id: true, sortOrder: true },
      });
      const sortOrderById = new Map(owned.map((g) => [g.id, g.sortOrder]));
      validateOrderSet(sortOrderById.keys(), order, {
        mismatchMessage: '排序列表须包含所有相册且不重复',
        mismatchCode: 'order_mismatch',
      });
      // manifestEtag 包含 group.sortOrder，所以只为位置真正变化的 group 重算；位置没动的跳过，
      // 避免 reorder 1 个 group 时把所有 group 的 manifest 都刷一遍（每次刷会扫该 group 全部 content）。
      const changed = order.filter((id, idx) => sortOrderById.get(id) !== idx);
      if (changed.length === 0) return;
      await bulkSetGroupSortOrder(tx, ownerUserId, order);
      await this.recomputeManifestEtagsAfterGroupReorder(tx, ownerUserId, changed);
    });
  }

  // ── 内部 helpers ──────────────────────────────────────────

  private async aggregateBytes(groupIds: string[]): Promise<Map<string, number>> {
    if (groupIds.length === 0) return new Map();
    const rows = await this.prisma.content.groupBy({
      by: ['groupId'],
      where: { groupId: { in: groupIds } },
      _sum: { imageSize: true, audioSize: true },
    });
    return sizeMapFromRows(rows);
  }

  private async aggregateBytesForOwner(ownerUserId: string): Promise<Map<string, number>> {
    const rows = await this.prisma.content.groupBy({
      by: ['groupId'],
      where: { group: { ownerUserId } },
      _sum: { imageSize: true, audioSize: true },
    });
    return sizeMapFromRows(rows);
  }

  async assertOwned(gid: string, ownerUserId: string): Promise<void> {
    const g = await this.prisma.group.findUnique({
      where: { id: gid },
      select: { ownerUserId: true },
    });
    if (!g || g.ownerUserId !== ownerUserId) {
      throw new NotFoundError('相册不存在');
    }
  }

  private queryOwnerGroups(
    ownerUserId: string,
    client: PrismaClientLike = this.prisma
  ): Promise<GroupListEntry[]> {
    return client.group.findMany({
      where: { ownerUserId },
      orderBy: [{ sortOrder: 'asc' }, { createdAt: 'asc' }],
      select: {
        id: true,
        name: true,
        structureEtag: true,
        manifestEtag: true,
        sortOrder: true,
        _count: { select: { contents: true } },
      },
    });
  }

  private async recomputeManifestEtagsAfterGroupReorder(
    tx: Prisma.TransactionClient,
    ownerUserId: string,
    changedGroupIds: string[]
  ): Promise<void> {
    const groups = await tx.group.findMany({
      where: { ownerUserId, id: { in: changedGroupIds } },
      select: {
        id: true,
        name: true,
        sortOrder: true,
        structureEtag: true,
        contents: {
          orderBy: { sortOrder: 'asc' },
          select: { id: true, contentEtag: true },
        },
      },
    });
    const manifestEtags = groups.map((group) => ({
      id: group.id,
      manifestEtag: computeGroupManifestEtag(group),
    }));
    await bulkSetGroupManifestEtags(tx, manifestEtags);
  }
}

function emptyCycle(): CycleResult {
  return {
    groupId: null,
    name: null,
    structureEtag: null,
    manifestEtag: null,
    sortOrder: null,
    contentCount: 0,
    position: null,
  };
}

function toSummary(
  g: {
    id: string;
    name: string;
    structureEtag: string;
    manifestEtag: string;
    sortOrder: number;
    _count: { contents: number };
  },
  totalBytes: number
): GroupSummaryT {
  return {
    id: g.id,
    name: g.name,
    structure_etag: g.structureEtag,
    manifest_etag: g.manifestEtag,
    sort_order: g.sortOrder,
    content_count: g._count.contents,
    total_bytes: totalBytes,
  };
}

function sizeMapFromRows(
  rows: Array<{
    groupId: string;
    _sum: { imageSize: number | null; audioSize: number | null };
  }>
): Map<string, number> {
  const result = new Map<string, number>();
  for (const row of rows) {
    result.set(row.groupId, (row._sum.imageSize ?? 0) + (row._sum.audioSize ?? 0));
  }
  return result;
}

async function bulkSetContentEtags(
  client: PrismaClientLike,
  updates: Array<{ id: string; etag: string }>
): Promise<void> {
  if (updates.length === 0) return;
  const ids = Prisma.join(updates.map((update) => update.id));
  await client.$executeRaw`
    UPDATE "contents"
    SET "content_etag" = CASE "id"
      ${Prisma.join(
        updates.map((update) => Prisma.sql`WHEN ${update.id} THEN ${update.etag}`),
        ' '
      )}
    END
    WHERE "id" IN (${ids})
  `;
}

async function bulkSetGroupManifestEtags(
  client: PrismaClientLike,
  updates: Array<{ id: string; manifestEtag: string }>
): Promise<void> {
  if (updates.length === 0) return;
  const ids = Prisma.join(updates.map((update) => update.id));
  await client.$executeRaw`
    UPDATE "groups"
    SET "manifest_etag" = CASE "id"
      ${Prisma.join(
        updates.map((update) => Prisma.sql`WHEN ${update.id} THEN ${update.manifestEtag}`),
        ' '
      )}
    END
    WHERE "id" IN (${ids})
  `;
}
