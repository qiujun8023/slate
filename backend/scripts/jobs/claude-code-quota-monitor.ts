#!/usr/bin/env bun
/**
 * 获取 Claude Code 限额数据，推送到 Slate 的 ai_quota_monitor 动态帧。
 *
 * 数据获取：
 *   1. Claude Code statusLine 模式：从 stdin 读取 rate_limits，输出状态栏并后台推送。
 *   2. 独立运行模式：发一次最小请求到 Anthropic API，从 headers 提取限额。
 *
 * 独立运行模式认证（按优先级）：
 *   1. 环境变量 ANTHROPIC_API_KEY（API key 或 OAuth Bearer token）
 *   2. 自动读取 ~/.claude/.credentials.json 中的 accessToken
 *
 * 环境变量：
 *   SLATE_API_BASE          Slate 后端地址，如 http://localhost:9494
 *   CLAUDE_QUOTA_CONTENT_ID Slate 中 ai_quota_monitor 类型动态帧的 contentId
 *   ANTHROPIC_API_KEY       （可选）Anthropic API key 或 OAuth token
 *   ANTHROPIC_API_BASE      （可选）API 地址，默认 https://api.anthropic.com
 *   CLAUDE_PLAN_LABEL       （可选）套餐标签，默认 "Max 20x"
 *   CLAUDE_QUOTA_PUSH_INTERVAL_MS （可选）statusLine 模式推送限频，默认 60000
 */

import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { spawn } from 'node:child_process';
import { IngestPayload, type DashboardDataPayloadT } from 'shared';
import {
  createScriptLogger,
  formatScriptError,
  readScriptErrorBody,
  truncateScriptLogText,
} from '../helpers/script-logger';
import { readPositiveIntEnv, requireEnv, stripTrailingSlash } from '../lib/env';
import type { SlateJob } from '../lib/job';
import {
  formatHourMinuteInTimeZone,
  formatMonthDayMinuteInTimeZone,
  readScriptTimeZone,
  sameLocalDateInTimeZone,
  weekdayIndexInTimeZone,
} from '../lib/time';

const logger = createScriptLogger('ClaudeCodeQuotaMonitor');

interface ClaudeQuotaMonitorConfig {
  slateAPIBase: string;
  contentID: string;
  anthropicAPIBase: string;
  planLabel: string;
  pushIntervalMs: number;
  logPath: string;
  lastPushTsPath: string;
  payloadPath: string;
  timeZone: string;
}

function readConfig(): ClaudeQuotaMonitorConfig {
  const cacheDir = join(homedir(), '.cache', 'slate-claude-quota');
  mkdirSync(cacheDir, { recursive: true });
  return {
    slateAPIBase: stripTrailingSlash(requireEnv('SLATE_API_BASE')),
    contentID: requireEnv('CLAUDE_QUOTA_CONTENT_ID'),
    anthropicAPIBase: stripTrailingSlash(
      process.env.ANTHROPIC_API_BASE ?? 'https://api.anthropic.com'
    ),
    planLabel: process.env.CLAUDE_PLAN_LABEL ?? 'Max 20x',
    pushIntervalMs: readPositiveIntEnv('CLAUDE_QUOTA_PUSH_INTERVAL_MS', 60_000),
    logPath: join(cacheDir, 'push.log'),
    lastPushTsPath: join(cacheDir, 'last-push-ts'),
    payloadPath: join(cacheDir, 'pending-payload.json'),
    timeZone: readScriptTimeZone(),
  };
}

function log(config: ClaudeQuotaMonitorConfig, msg: string) {
  try {
    appendFileSync(config.logPath, `[${new Date().toISOString()}] ${msg}\n`);
  } catch {
    // Logging must never break the statusLine command.
  }
}

// ── 认证：自动读取 OAuth token ──────────────────────────────────────

function resolveAuthToken(): string {
  const envKey = process.env.ANTHROPIC_API_KEY;
  if (envKey) return envKey;

  const credPath = join(homedir(), '.claude', '.credentials.json');
  if (!existsSync(credPath)) {
    logger.error('No ANTHROPIC_API_KEY set and ~/.claude/.credentials.json was not found.');
    process.exit(1);
  }

  try {
    const creds = JSON.parse(readFileSync(credPath, 'utf-8'));
    const token = creds?.accessToken;
    if (!token) {
      logger.error('No accessToken was found in .credentials.json.');
      process.exit(1);
    }
    return token;
  } catch (e) {
    logger.error(`Failed to read .credentials.json: ${formatScriptError(e)}`);
    process.exit(1);
  }
}

// ── Claude Code statusLine stdin ───────────────────────────────────

interface ClaudeCodeStatusLineInput {
  rate_limits?: {
    five_hour?: { used_percentage?: number; resets_at?: number };
    seven_day?: { used_percentage?: number; resets_at?: number };
  };
}

interface QuotaLimitSnapshot {
  pct5h: number;
  reset5h: number;
  pct7d: number;
  reset7d: number;
}

async function readStatusLineInput(): Promise<ClaudeCodeStatusLineInput | null> {
  if (process.stdin.isTTY) return null;
  try {
    const raw = (await readStdinWithTimeout(500)).trim();
    if (!raw) return null;
    const parsed = JSON.parse(raw) as ClaudeCodeStatusLineInput;
    return parsed.rate_limits ? parsed : null;
  } catch {
    return null;
  }
}

async function readStdinWithTimeout(ms: number): Promise<string> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = [];
    const timer = setTimeout(() => {
      process.stdin.destroy();
      resolve(Buffer.concat(chunks).toString('utf-8'));
    }, ms);
    process.stdin.on('data', (chunk: Buffer) => chunks.push(chunk));
    process.stdin.on('end', () => {
      clearTimeout(timer);
      resolve(Buffer.concat(chunks).toString('utf-8'));
    });
    process.stdin.on('error', () => {
      clearTimeout(timer);
      resolve(Buffer.concat(chunks).toString('utf-8'));
    });
  });
}

function rateLimitsFromStatusLine(input: ClaudeCodeStatusLineInput): QuotaLimitSnapshot | null {
  const rl = input.rate_limits;
  if (!rl) return null;
  return {
    pct5h: clampPercent(rl.five_hour?.used_percentage ?? 0),
    reset5h: readUnixSeconds(rl.five_hour?.resets_at),
    pct7d: clampPercent(rl.seven_day?.used_percentage ?? 0),
    reset7d: readUnixSeconds(rl.seven_day?.resets_at),
  };
}

// ── Anthropic API 探测 ─────────────────────────────────────────────

interface UnifiedRateLimit {
  utilization5h: number; // 0.0 - 1.0
  reset5h: number; // Unix timestamp (seconds)
  utilization7d: number; // 0.0 - 1.0
  reset7d: number; // Unix timestamp (seconds)
  status: string; // allowed | allowed_warning | rejected
  bindingWindow: string; // five_hour | seven_day | ...
}

async function probeUnifiedRateLimit(
  config: ClaudeQuotaMonitorConfig,
  token: string
): Promise<UnifiedRateLimit | null> {
  try {
    const res = await fetch(`${config.anthropicAPIBase}/v1/messages`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'anthropic-version': '2023-06-01',
        'anthropic-beta': 'oauth-2025-04-20',
        'content-type': 'application/json',
        'User-Agent': 'claude-code/2.1.5',
      },
      body: JSON.stringify({
        model: 'claude-haiku-4-5-20251001',
        max_tokens: 1,
        messages: [{ role: 'user', content: 'quota' }],
      }),
    });

    const h = res.headers;

    // unified headers（Pro/Max 计划返回这些）
    const u5h = h.get('anthropic-ratelimit-unified-5h-utilization');
    const r5h = h.get('anthropic-ratelimit-unified-5h-reset');
    const u7d = h.get('anthropic-ratelimit-unified-7d-utilization');
    const r7d = h.get('anthropic-ratelimit-unified-7d-reset');
    const status = h.get('anthropic-ratelimit-unified-status') ?? '';
    const binding = h.get('anthropic-ratelimit-unified-representative-claim') ?? '';

    if (u5h != null && r5h != null) {
      return {
        utilization5h: parseFloat(u5h),
        reset5h: parseInt(r5h, 10),
        utilization7d: u7d != null ? parseFloat(u7d) : 0,
        reset7d: r7d != null ? parseInt(r7d, 10) : 0,
        status,
        bindingWindow: binding,
      };
    }

    // Fallback: legacy headers（API key 用户可能返回这些）
    const rl = h.get('anthropic-ratelimit-requests-limit');
    const rr = h.get('anthropic-ratelimit-requests-remaining');
    const rs = h.get('anthropic-ratelimit-requests-reset');

    if (rl && rr) {
      const limit = parseInt(rl, 10);
      const remaining = parseInt(rr, 10);
      const utilization = limit > 0 ? (limit - remaining) / limit : 0;
      const resetAt = rs ? Math.floor(new Date(rs).getTime() / 1000) : 0;
      return {
        utilization5h: utilization,
        reset5h: resetAt,
        utilization7d: 0,
        reset7d: 0,
        status: '',
        bindingWindow: 'five_hour',
      };
    }

    return null;
  } catch (e) {
    logger.warn(`Anthropic API probe failed: ${formatScriptError(e)}`);
    return null;
  }
}

// ── 格式化 ─────────────────────────────────────────────────────────

const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

function readUnixSeconds(value: unknown): number {
  const n = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
  return Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0;
}

function clampPercent(value: number): number {
  return Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
}

function formatReset5h(sec: number, timeZone: string): string {
  if (!sec) return '--';
  const d = new Date(sec * 1000);
  const now = new Date();
  const sameDay = sameLocalDateInTimeZone(d, now, timeZone);
  return `${sameDay ? '' : '明 '}${formatHourMinuteInTimeZone(d, timeZone)}`;
}

function formatReset7d(sec: number, timeZone: string): string {
  if (!sec) return '--';
  const d = new Date(sec * 1000);
  return `${WEEKDAYS[weekdayIndexInTimeZone(d, timeZone)]} ${formatHourMinuteInTimeZone(
    d,
    timeZone
  )}`;
}

function nowLabel(timeZone: string): string {
  return formatMonthDayMinuteInTimeZone(new Date(), timeZone);
}

function bar(pct: number): string {
  const n = Math.max(0, Math.min(10, Math.round(pct / 10)));
  return '▓'.repeat(n) + '░'.repeat(10 - n);
}

function formatStatusLine(limits: QuotaLimitSnapshot, timeZone: string): string {
  return `5h ${bar(limits.pct5h)} ${Math.round(limits.pct5h)}% ${formatReset5h(limits.reset5h, timeZone)} | 7d ${bar(limits.pct7d)} ${Math.round(limits.pct7d)}% ${formatReset7d(limits.reset7d, timeZone)}`;
}

function statusLabelForLimits(
  primaryPercent: number,
  secondaryPercent: number,
  apiStatus?: string
): string {
  if (apiStatus === 'rejected' || primaryPercent >= 100 || secondaryPercent >= 100) {
    return '已限流';
  }
  if (apiStatus === 'allowed_warning' || primaryPercent >= 80 || secondaryPercent >= 80) {
    return '警告';
  }
  return '正常';
}

// ── 主流程 ─────────────────────────────────────────────────────────

interface QuotaData extends Record<string, unknown> {
  service_label: string;
  plan_label: string;
  status_label: string;
  primary_window_label: string;
  primary_used_percent: number;
  primary_reset_at_label: string;
  secondary_window_label: string;
  secondary_used_percent: number;
  secondary_reset_at_label: string;
  updated_label: string;
}

function buildQuotaDataFromLimits(
  config: ClaudeQuotaMonitorConfig,
  limits: QuotaLimitSnapshot,
  apiStatus?: string
): QuotaData {
  const primaryPercent = Math.round(clampPercent(limits.pct5h));
  const secondaryPercent = Math.round(clampPercent(limits.pct7d));
  return {
    service_label: 'Claude Code',
    plan_label: config.planLabel,
    status_label: statusLabelForLimits(primaryPercent, secondaryPercent, apiStatus),
    primary_window_label: '5h窗口',
    primary_used_percent: primaryPercent,
    primary_reset_at_label: formatReset5h(limits.reset5h, config.timeZone),
    secondary_window_label: '周限额',
    secondary_used_percent: secondaryPercent,
    secondary_reset_at_label: formatReset7d(limits.reset7d, config.timeZone),
    updated_label: nowLabel(config.timeZone),
  };
}

async function buildQuotaDataFromProbe(config: ClaudeQuotaMonitorConfig): Promise<QuotaData> {
  const token = resolveAuthToken();
  const rl = await probeUnifiedRateLimit(config, token);

  if (!rl) {
    return {
      ...buildQuotaDataFromLimits(config, { pct5h: 0, reset5h: 0, pct7d: 0, reset7d: 0 }),
      status_label: '未知',
    };
  }

  return buildQuotaDataFromLimits(
    config,
    {
      pct5h: rl.utilization5h * 100,
      reset5h: rl.reset5h,
      pct7d: rl.utilization7d * 100,
      reset7d: rl.reset7d,
    },
    rl.status
  );
}

function buildPayload(data: DashboardDataPayloadT): string {
  return JSON.stringify(IngestPayload.parse({ version: 1, data }));
}

function shouldPush(config: ClaudeQuotaMonitorConfig): boolean {
  try {
    return (
      Date.now() - Number(readFileSync(config.lastPushTsPath, 'utf-8').trim()) >=
      config.pushIntervalMs
    );
  } catch {
    return true;
  }
}

async function pushPayloadBody(config: ClaudeQuotaMonitorConfig, body: string) {
  const url = `${config.slateAPIBase}/api/v1/contents/${config.contentID}/data`;
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
  });

  if (!res.ok) {
    const body = await readScriptErrorBody(res);
    throw new Error(`Slate push failed ${res.status}: ${body}`);
  }

  const result = await res.json();
  logger.info(
    `Slate accepted Claude Code quota push: ${truncateScriptLogText(JSON.stringify(result), 1000)}`
  );
}

async function push(config: ClaudeQuotaMonitorConfig, data: DashboardDataPayloadT) {
  await pushPayloadBody(config, buildPayload(data));
}

async function pushPendingPayload(config: ClaudeQuotaMonitorConfig) {
  try {
    await pushPayloadBody(config, readFileSync(config.payloadPath, 'utf-8'));
    log(config, 'ok');
  } catch (e) {
    log(config, `error ${e}`);
  }
}

function spawnBackgroundPush() {
  const child = spawn(process.execPath, [import.meta.filename, '--push'], {
    detached: true,
    stdio: 'ignore',
  });
  child.unref();
}

function scheduleStatusLinePush(
  config: ClaudeQuotaMonitorConfig,
  data: DashboardDataPayloadT,
  limits: QuotaLimitSnapshot
) {
  if (!shouldPush(config)) return;
  writeFileSync(config.lastPushTsPath, String(Date.now()));
  writeFileSync(config.payloadPath, buildPayload(data));
  spawnBackgroundPush();
  log(config, `push 5h=${Math.round(limits.pct5h)}% 7d=${Math.round(limits.pct7d)}%`);
}

export async function runClaudeCodeQuotaMonitorJob(): Promise<void> {
  const config = readConfig();
  const quota = await buildQuotaDataFromProbe(config);
  logger.info(`Claude Code quota data: ${truncateScriptLogText(JSON.stringify(quota), 1000)}`);
  await push(config, quota);
}

async function main() {
  const config = readConfig();
  if (process.argv[2] === '--push') {
    await pushPendingPayload(config);
    return;
  }

  const statusLineInput = await readStatusLineInput();
  const statusLineLimits = statusLineInput ? rateLimitsFromStatusLine(statusLineInput) : null;
  if (statusLineLimits) {
    process.stdout.write(formatStatusLine(statusLineLimits, config.timeZone));
    scheduleStatusLinePush(
      config,
      buildQuotaDataFromLimits(config, statusLineLimits),
      statusLineLimits
    );
    return;
  }

  await runClaudeCodeQuotaMonitorJob();
}

export const job: SlateJob = {
  id: 'claude-code-quota-monitor',
  description: 'Fetch Claude Code quota usage and push it to a Slate dashboard frame.',
  run: runClaudeCodeQuotaMonitorJob,
};

if (import.meta.main) {
  main().catch((e) => {
    logger.error(`Claude Code quota monitor failed: ${formatScriptError(e)}`);
    process.exit(1);
  });
}
