import { DEFAULT_TTS_VOICE, catalog, type DynamicConfig, type DynamicType } from '@/api';

const LOCAL_TIME_ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

export function defaultConfig(type: DynamicType): DynamicConfig {
  const audio = { audioEnabled: false, audioVoice: DEFAULT_TTS_VOICE };
  switch (type) {
    case 'daily_calendar':
    case 'month_calendar':
      return { type, tz: LOCAL_TIME_ZONE, ...audio };
    case 'weather':
      return {
        type,
        tz: LOCAL_TIME_ZONE,
        locationId: '101010100',
        locationLabel: '北京',
        refreshIntervalSec: 600,
        ...audio,
      };
    case 'history_today':
      return { type, tz: LOCAL_TIME_ZONE, source: 'wikipedia', ...audio };
    case 'weather_alert':
      return { type, province: '', refreshIntervalSec: 600, ...audio };
    case 'earthquake_report':
      return { type, refreshIntervalSec: 600, ...audio };
    case 'dashboard':
      return {
        type,
        template: { kind: 'custom', template: catalog.dashboardStarter },
        refreshIntervalSec: 600,
      };
    case 'font_test':
      return { type, fontId: 'fusion_pixel_12', invert: false };
    case 'hot_list':
      return { type, source: 'weibo', refreshIntervalSec: 600 };
  }
}
