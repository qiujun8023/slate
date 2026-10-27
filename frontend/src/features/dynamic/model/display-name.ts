import {
  fontInfo,
  hotListSource,
  normalizeWeatherAlertProvince,
  systemTemplate,
  type DynamicConfig,
  type DynamicType,
} from '@/api';
import { zonedDateParts } from '@/features/dynamic/lib/date';

export function defaultDynamicFrameName(type: DynamicType, config: DynamicConfig): string {
  switch (type) {
    case 'daily_calendar':
      return '日历';
    case 'month_calendar':
      return '月历';
    case 'weather':
      return config.locationLabel ? `${config.locationLabel}天气` : '天气';
    case 'history_today':
      return '历史上的今天';
    case 'weather_alert':
      return dynamicStatusTitle(config) ?? '气象预警';
    case 'earthquake_report':
      return '地震速报';
    case 'dashboard':
      return dashboardStatusTitle(config);
    case 'font_test':
      return dynamicStatusTitle(config) ?? '字体测试';
    case 'hot_list':
      return dynamicStatusTitle(config) ?? '热榜';
  }
}

export function effectiveDynamicFrameName(
  type: DynamicType,
  config: DynamicConfig,
  frameName: string
): string {
  if (type === 'dashboard') return frameName.trim() || defaultDynamicFrameName(type, config);
  return defaultDynamicFrameName(type, config);
}

export function effectiveDynamicStatusBarText(
  type: DynamicType | null,
  config: DynamicConfig | null,
  frameName: string
): string | null {
  if (type === 'dashboard') {
    return frameName.trim() || (config ? dashboardStatusTitle(config) : '外部数据');
  }
  return dynamicStatusTitle(config);
}

export function frameNameForSyncedDynamicConfigChange(
  previous: DynamicConfig,
  next: DynamicConfig
): string | null {
  if (
    next.type === 'weather' &&
    previous.type === 'weather' &&
    next.locationLabel !== previous.locationLabel
  ) {
    return defaultDynamicFrameName(next.type, next);
  }
  if (
    next.type === 'weather_alert' &&
    previous.type === 'weather_alert' &&
    next.province !== previous.province
  ) {
    return defaultDynamicFrameName(next.type, next);
  }
  return null;
}

export function dynamicStatusTitle(config: DynamicConfig | null | undefined): string | null {
  if (!config) return null;

  switch (config.type) {
    case 'daily_calendar': {
      const date = zonedDateParts(new Date(), config.tz ?? 'Asia/Shanghai');
      return `${date.month}月${date.day}日`;
    }
    case 'month_calendar': {
      const date = zonedDateParts(new Date(), config.tz ?? 'Asia/Shanghai');
      return `${date.year}年${date.month}月`;
    }
    case 'weather':
      return config.locationLabel ? `${config.locationLabel}天气` : '天气';
    case 'history_today': {
      const date = zonedDateParts(new Date(), config.tz ?? 'Asia/Shanghai');
      return `历史上的${date.month}月${date.day}日`;
    }
    case 'weather_alert':
      return `${normalizeWeatherAlertProvince(config.province) || '全国'}气象预警`;
    case 'earthquake_report':
      return '地震速报';
    case 'dashboard':
      return dashboardStatusTitle(config);
    case 'font_test':
      return fontInfo(config.fontId)?.label ?? '字体测试';
    case 'hot_list':
      return `${hotListSource(config.source)?.shortLabel ?? ''}热榜`;
  }
}

export function dashboardStatusTitle(config: DynamicConfig): string {
  const template = config.template;
  if (template?.kind === 'system') return systemTemplate(template.id)?.label ?? '外部数据';
  if (template?.template) return template.template.name?.trim() || '自定义模板';
  return '外部数据';
}
