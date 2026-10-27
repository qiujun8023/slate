// Wire types and fixed choices, both generated from the Go backend (go generate ./internal/api).
import catalogJson from './catalog.json';
import type { Catalog, DynamicConfig, DynamicType } from './generated';

export * from './generated';

export const catalog = catalogJson as Catalog;

export const TTS_VOICES = catalog.ttsVoices;
export const DEFAULT_TTS_VOICE = catalog.defaultTtsVoice;

export function isTtsVoice(value: string): boolean {
  return TTS_VOICES.includes(value);
}

const AUDIO_TYPES: readonly DynamicType[] = [
  'daily_calendar',
  'month_calendar',
  'weather',
  'history_today',
  'weather_alert',
  'earthquake_report',
];

export function supportsAudio(type: DynamicType): boolean {
  return AUDIO_TYPES.includes(type);
}

export function isAudioDynamicConfig(config: DynamicConfig): boolean {
  return supportsAudio(config.type);
}

export function hotListSource(id: string | undefined) {
  return catalog.hotListSources.find((source) => source.id === id);
}

export function fontInfo(id: string | undefined) {
  return catalog.fonts.find((font) => font.id === id);
}

export function systemTemplate(id: string | undefined) {
  return catalog.dashboardTemplates.find((template) => template.id === id);
}

/** Maps aliases like "广西" to the full province name, or "" for nationwide. */
export function normalizeWeatherAlertProvince(value: string | undefined): string {
  const text = (value ?? '').trim();
  if (!text || text === '全国' || text === '全部' || text === '中国') return '';
  for (const province of catalog.weatherAlertProvinces) {
    const short = province.replace(/省$|市$/, '').replace(/(?:壮族|回族|维吾尔)?自治区$/, '');
    if (text === province || text === short) return province;
  }
  return '';
}
