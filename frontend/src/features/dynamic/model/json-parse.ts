import type { DashboardTemplate, DynamicConfig } from '@/api';
import { canonicalJsonKey } from '@/features/dynamic/lib/json';

/** Checks the template shape; field-level validation happens on the server preview. */
export function parseDashboardTemplate(
  text: string
): { ok: true; template: DashboardTemplate } | { ok: false; error: string } {
  const parsed = parseJsonRecord(text);
  if (!parsed.ok) return parsed;
  const blocks = parsed.data.blocks;
  if (!Array.isArray(blocks) || blocks.length === 0 || blocks.length > 32) {
    return { ok: false, error: '模板须包含 1-32 个区块（blocks）' };
  }
  return { ok: true, template: { version: 1, ...parsed.data } as DashboardTemplate };
}

export function parseJsonRecord(
  text: string
): { ok: true; data: Record<string, unknown> } | { ok: false; error: string } {
  const parsed = parseJson(text);
  if (!parsed.ok) return parsed;
  if (!parsed.data || typeof parsed.data !== 'object' || Array.isArray(parsed.data)) {
    return { ok: false, error: '必须是 JSON object' };
  }
  return { ok: true, data: parsed.data as Record<string, unknown> };
}

export function parseJson(
  text: string
): { ok: true; data: unknown } | { ok: false; error: string } {
  try {
    return { ok: true, data: JSON.parse(text) };
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : 'JSON 解析失败' };
  }
}

export function dynamicConfigKey(config: DynamicConfig): string {
  return canonicalJsonKey(config);
}
