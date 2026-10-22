import { Injectable, Logger } from '@nestjs/common';
import { randomBytes } from 'node:crypto';
import { Prisma } from '@prisma/client';
import { MacAddress, type DeviceStateT } from 'shared';
import { ConflictError, NotFoundError } from '../../common/errors';
import { prismaUniqueTargetIncludes, type PrismaClientLike } from '../../common/db/prisma-utils';
import {
  DeviceSecretAuthCacheService,
  hashDeviceSecret,
} from '../../infra/auth/device-secret-auth-cache.service';
import { PrismaService } from '../../infra/prisma/prisma.service';
import { DeviceCurrentContentService } from '../contents/device-current-content.service';
import { GroupsService, type CycleResult } from '../groups/groups.service';
import type { DevicePollSnapshot, TelemetryInput } from './device-types';
import { toDeviceStatePayload } from './device-state.presenter';
import { PairCodeService } from './pair-code.service';

const REGISTER_RESET_THROTTLE_MS = 60_000;

interface RegisterResetOutcome {
  deviceId: string;
  reclaimed: boolean;
  previousOwnerUserId: string | null;
  previousSecretHash: string | null;
  isFirstRegister: boolean;
}

interface ExistingRegisterDevice {
  id: string;
  mac: string;
  ownerUserId: string | null;
  lastRegisteredAt: Date | null;
  secretHash: string | null;
}

@Injectable()
export class DeviceFirmwareService {
  private readonly logger = new Logger(DeviceFirmwareService.name);

  constructor(
    private readonly prisma: PrismaService,
    private readonly groups: GroupsService,
    private readonly deviceSecrets: DeviceSecretAuthCacheService,
    private readonly currentContent: DeviceCurrentContentService,
    private readonly pairCodes: PairCodeService
  ) {}

  // 无鉴权路由唯一行为：固件 NVS 没 secret 时调一次。
  //
  // ⚠️ 设计决定（不要改）：同 mac 二次进来一律走 reset 路径，清 owner、清相册、轮换
  // secret + pair_code —— 即「物理重置即转移」语义。已知代价是 mac 可被嗅探/伪造，
  // 攻击者拿到 mac 即可夺取设备所有权；当前用户基数极小（个位数），权衡后选了可用性：
  //   - 设备 NVS 损坏、刷固件、二手转手、忘记 owner 账号密码 等场景下不会把用户锁死；
  //   - 持有者本人想换网络/重置配对，无需先登 Web 端解绑，物理控制即可。
  // 60s 节流仅用于防一次 NVS 写盘失败被放大成连续 reset 丢所有权，不是安全机制。
  //
  // 请勿"修复"成 require-unbind（已绑定就 409）：上一版试过，会把"NVS 坏掉的合法持有者"
  // 永久锁在 409 错误里，必须先能登 Web 端才能解开 —— 反而让设备失去物理控制权这条最后
  // 兜底。若未来用户量上来要堵 mac 后门，应通过物理重置 + 屏显 challenge + Web 端确认
  // 的方式重做，而不是简单拒绝 mac-only 注册。
  async registerOrReset(mac: string): Promise<{
    deviceId: string;
    deviceSecret: string;
    pairCode: string;
    reclaimed: boolean;
    serverTime: string;
  }> {
    const normalizedMac = MacAddress.parse(mac);
    const now = new Date();

    // 预检：节流命中时直接拒绝，避免白白消耗熵 + 一次 pair_code 唯一性查询。
    // 事务内持 device 行锁后还会复检一次，保证并发 reset 不会绕过节流。
    const preExisting = await this.findRegisterDeviceByMac(this.prisma, normalizedMac);
    if (preExisting?.lastRegisteredAt) {
      const elapsedMs = now.getTime() - preExisting.lastRegisteredAt.getTime();
      if (elapsedMs < REGISTER_RESET_THROTTLE_MS) {
        throw throttleError(elapsedMs);
      }
    }

    const outcome = await this.prisma
      .$transaction(
        async (tx): Promise<RegisterResetOutcome & { secret: string; pairCode: string }> => {
          await lockDeviceMacRow(tx, normalizedMac);
          const current = await this.findRegisterDeviceByMac(tx, normalizedMac);
          if (!current) {
            const secret = generateSecret();
            const secretHash = hashDeviceSecret(secret);
            const pairCode = await this.pairCodes.generateUniquePairCode(tx);
            const created = await tx.device.create({
              data: { mac: normalizedMac, secretHash, pairCode, lastRegisteredAt: now },
              select: { id: true },
            });
            return {
              deviceId: created.id,
              reclaimed: false,
              previousOwnerUserId: null,
              previousSecretHash: null,
              isFirstRegister: true,
              secret,
              pairCode,
            };
          }

          const elapsedMs = current.lastRegisteredAt
            ? now.getTime() - current.lastRegisteredAt.getTime()
            : Number.POSITIVE_INFINITY;
          if (elapsedMs < REGISTER_RESET_THROTTLE_MS) {
            throw throttleError(elapsedMs);
          }
          const secret = generateSecret();
          const secretHash = hashDeviceSecret(secret);
          const pairCode = await this.pairCodes.generateUniquePairCode(tx);
          await tx.device.update({
            where: { id: current.id },
            data: {
              mac: normalizedMac,
              secretHash,
              pairCode,
              ownerUserId: null,
              selectedGroupId: null,
              lastRegisteredAt: now,
            },
          });
          return {
            deviceId: current.id,
            reclaimed: current.ownerUserId !== null,
            previousOwnerUserId: current.ownerUserId,
            previousSecretHash: current.secretHash,
            isFirstRegister: false,
            secret,
            pairCode,
          };
        }
      )
      .catch((err: unknown) => {
        // 并发场景：两个同 mac 的 register 都看到 current=null，第一个 create 后第二个撞 mac
        // 唯一约束 P2002。让客户端短延迟重试，避免在 reset 路径上暴露 5xx。
        if (
          err instanceof Prisma.PrismaClientKnownRequestError &&
          err.code === 'P2002' &&
          prismaUniqueTargetIncludes(err, 'mac')
        ) {
          throw new ConflictError('设备并发注册中，请重试', {
            code: 'register_race',
            retry_after_sec: 1,
          });
        }
        throw err;
      });

    if (outcome.isFirstRegister) {
      this.logger.log(`Device ${outcome.deviceId} was first registered from MAC ${normalizedMac}.`);
    } else if (outcome.reclaimed) {
      this.logger.log(
        `Device ${outcome.deviceId} was reclaimed by physical reset from MAC ${normalizedMac}; previous owner was ${outcome.previousOwnerUserId ?? 'none'}.`
      );
    } else {
      this.logger.log(
        `Device ${outcome.deviceId} was re-registered from MAC ${normalizedMac} while unowned.`
      );
    }
    this.deviceSecrets.invalidateHash(outcome.previousSecretHash);

    return {
      deviceId: outcome.deviceId,
      deviceSecret: outcome.secret,
      pairCode: outcome.pairCode,
      reclaimed: outcome.reclaimed,
      serverTime: now.toISOString(),
    };
  }

  async recordTelemetry(
    deviceId: string,
    t: TelemetryInput | undefined
  ): Promise<DevicePollSnapshot> {
    try {
      return await this.prisma.device.update({
        where: { id: deviceId },
        data: {
          lastSeenAt: new Date(),
          ...(t?.battery_pct !== undefined ? { batteryPct: t.battery_pct } : {}),
          ...(t?.rssi_dbm !== undefined ? { rssiDbm: t.rssi_dbm } : {}),
          ...(t?.fw_version !== undefined ? { fwVersion: t.fw_version } : {}),
        },
        select: {
          id: true,
          mac: true,
          name: true,
          ownerUserId: true,
          selectedGroupId: true,
          pairCode: true,
          selectedGroup: { select: { manifestEtag: true } },
        },
      });
    } catch (err) {
      if (err instanceof Prisma.PrismaClientKnownRequestError && err.code === 'P2025') {
        throw new NotFoundError('设备不存在');
      }
      throw err;
    }
  }

  async poll(deviceId: string, telemetry: TelemetryInput | undefined): Promise<DeviceStateT> {
    const device = await this.recordTelemetry(deviceId, telemetry);
    const currentFrame = await this.currentContent.resolveCurrentContentRequest(device, telemetry);
    const resolvedCurrentFrame =
      telemetry?.wake_reason === 'timer'
        ? await this.currentContent.refreshCurrentContentForDeviceIfDue(currentFrame, device)
        : currentFrame;
    const state = await this.buildState(deviceId, { device });
    if (
      state.group &&
      resolvedCurrentFrame &&
      state.group.id === resolvedCurrentFrame.groupId &&
      state.group.manifest_etag === resolvedCurrentFrame.manifestEtag
    ) {
      state.current_content = this.currentContent.currentContentForDevice(resolvedCurrentFrame);
    } else {
      state.current_content = null;
    }
    return state;
  }

  async buildState(
    deviceId: string,
    cached?: { group?: CycleResult; device?: DevicePollSnapshot }
  ): Promise<DeviceStateT> {
    const device =
      cached?.device !== undefined
        ? cached.device
        : await this.prisma.device.findUnique({
            where: { id: deviceId },
            select: {
              id: true,
              mac: true,
              name: true,
              ownerUserId: true,
              selectedGroupId: true,
              pairCode: true,
              selectedGroup: { select: { manifestEtag: true } },
            },
          });
    if (!device) {
      throw new NotFoundError(`device ${deviceId} disappeared mid-request`);
    }
    const resolvedGroup =
      cached?.group !== undefined
        ? cached.group
        : await this.groups.describeDeviceGroupSnapshot(device);

    return toDeviceStatePayload(device, resolvedGroup);
  }

  private async findRegisterDeviceByMac(
    client: PrismaClientLike,
    normalizedMac: string
  ): Promise<ExistingRegisterDevice | null> {
    const select = {
      id: true,
      mac: true,
      ownerUserId: true,
      lastRegisteredAt: true,
      secretHash: true,
    } as const satisfies Prisma.DeviceSelect;
    return client.device.findUnique({ where: { mac: normalizedMac }, select });
  }
}

// secret = 32B 随机熵 hex 编码（64 字符），设备 NVS 持久化，DB 只存 sha256(secret) 比对。
function generateSecret(): string {
  return randomBytes(32).toString('hex');
}

function throttleError(elapsedMs: number): ConflictError {
  const retryAfterSec = Math.max(Math.ceil((REGISTER_RESET_THROTTLE_MS - elapsedMs) / 1000), 1);
  return new ConflictError('设备刚刚注册过，请稍后再重置', {
    code: 'register_throttled',
    retry_after_sec: retryAfterSec,
  });
}

async function lockDeviceMacRow(tx: Prisma.TransactionClient, mac: string): Promise<void> {
  await tx.$queryRaw<Array<{ id: string }>>`
    SELECT id FROM "devices" WHERE mac = ${mac} FOR UPDATE
  `;
}
