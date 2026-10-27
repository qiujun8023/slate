import {
  Activity,
  BarChart3,
  Bell,
  BookText,
  Calendar,
  CalendarDays,
  CloudSun,
  Flame,
  Type as TypeIcon,
  type LucideIcon,
} from 'lucide-react';
import type { DynamicType } from '@/api';

export interface DynamicTypeMeta {
  label: string;
  hint: string;
  description: string;
  hasConfigurableParams: boolean;
  supportsAudio: boolean;
  Icon: LucideIcon;
}

export const DYNAMIC_TYPE_META = {
  daily_calendar: {
    label: '日历',
    hint: '日期 · 星期 · 农历 · 节气',
    description: '显示今日公历、农历与干支。',
    hasConfigurableParams: false,
    supportsAudio: true,
    Icon: Calendar,
  },
  month_calendar: {
    label: '月历',
    hint: '整月日期 · 农历 · 节日',
    description: '显示当月日历。',
    hasConfigurableParams: false,
    supportsAudio: true,
    Icon: CalendarDays,
  },
  weather: {
    label: '天气',
    hint: '实时气温 / 湿度 / 风速',
    description: '按城市显示实时天气。数据来自 QWeather。',
    hasConfigurableParams: true,
    supportsAudio: true,
    Icon: CloudSun,
  },
  history_today: {
    label: '历史上的今天',
    hint: '今日历史大事，每日 0 点更新',
    description: '自动显示今日历史事件，可选维基百科或百度百科。',
    hasConfigurableParams: true,
    supportsAudio: true,
    Icon: BookText,
  },
  weather_alert: {
    label: '气象预警',
    hint: '中央气象台 · 全国预警',
    description: '显示中央气象台全国或指定省份气象预警。',
    hasConfigurableParams: true,
    supportsAudio: true,
    Icon: Bell,
  },
  earthquake_report: {
    label: '地震速报',
    hint: '中国地震台网 · 最新速报',
    description: '显示中国地震台网最新地震速报。',
    hasConfigurableParams: true,
    supportsAudio: true,
    Icon: Activity,
  },
  dashboard: {
    label: '外部数据',
    hint: '模板 + JSON 数据推送',
    description: '选择系统模板或自定义模板，后续只推送数据即可刷新画面。',
    hasConfigurableParams: true,
    supportsAudio: false,
    Icon: BarChart3,
  },
  font_test: {
    label: '字体测试',
    hint: '切换字体 · 查看 1bpp 字形',
    description: '测试 Fusion Pixel 字体在墨水屏上的渲染。',
    hasConfigurableParams: true,
    supportsAudio: false,
    Icon: TypeIcon,
  },
  hot_list: {
    label: '热榜',
    hint: '微博 / 知乎 / B站等榜单',
    description: '选择站点热榜，自动刷新并以墨水屏列表展示。',
    hasConfigurableParams: true,
    supportsAudio: false,
    Icon: Flame,
  },
} satisfies Record<DynamicType, DynamicTypeMeta>;

export const DYNAMIC_TYPE_ORDER = [
  'daily_calendar',
  'month_calendar',
  'history_today',
  'weather',
  'weather_alert',
  'earthquake_report',
  'hot_list',
  'dashboard',
  'font_test',
] as const satisfies readonly DynamicType[];
