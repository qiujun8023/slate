// 外部数据模板的示例数据：新建时预填，用户改成自己的数据后推送。

export const DASHBOARD_CUSTOM_STARTER_SAMPLE_DATA = {
  primary_label: '收入',
  primary_value: '128k',
  primary_trend: [3, 8, 5, 13, 21, 18, 26],
  secondary_label: '请求',
  secondary_value: '42.8k',
  third_label: '转化率',
  third_value: '12.6%',
  fourth_label: '延迟',
  fourth_value: '183ms',
  primary_progress_label: '目标',
  primary_progress_percent: 72,
  primary_progress_text: '72%',
  secondary_progress_label: '健康',
  secondary_progress_percent: 91,
  secondary_progress_text: '91%',
  footer_left: '05-27 16:30',
  footer_right: '业务看板',
} as const;

export const DASHBOARD_AI_QUOTA_MONITOR_SAMPLE_DATA = {
  service_label: 'Claude Code',
  plan_label: 'Pro',
  status_label: '正常',
  primary_window_label: '5小时',
  primary_used_percent: 68,
  primary_reset_at_label: '05-27 20:00',
  secondary_window_label: '每周',
  secondary_used_percent: 41,
  secondary_reset_at_label: '06-03 08:00',
  updated_label: '05-27 16:30',
} as const;

export const DASHBOARD_SYSTEM_TEMPLATE_SAMPLE_DATA: Record<string, Record<string, unknown>> = {
  ai_quota_monitor: DASHBOARD_AI_QUOTA_MONITOR_SAMPLE_DATA,
};
