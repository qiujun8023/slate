import type { DynamicConfigT } from 'shared';

export type DynamicConfigChange = (config: DynamicConfigT) => void;

export type AudioDynamicConfig = Extract<
  DynamicConfigT,
  {
    type:
      | 'daily_calendar'
      | 'month_calendar'
      | 'weather'
      | 'history_today'
      | 'weather_alert'
      | 'earthquake_report';
  }
>;

export type RefreshableDynamicConfig =
  AudioDynamicConfig | Extract<DynamicConfigT, { type: 'hot_list' | 'dashboard' }>;
