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

export const DASHBOARD_AI_USAGE_STATS_SAMPLE_DATA = {
  today_cost_usd: 155.7732,
  today_request_count: 1602,
  today_token_count: 172500000,
  today_cache_hit_rate_percent: 93.2,
  total_cost_usd: 1860.1969,
  total_request_count: 20305,
  total_token_count: 2102300000,
  total_cache_hit_rate_percent: 94.0,
  average_latency_ms: 17270,
  today_cost_per_million_tokens_usd: 0.9,
  total_cost_per_million_tokens_usd: 0.88,
  last_updated_time_label: '16:30',
  last_updated_label: '05-26 16:30',
  platform_breakdown: [
    {
      platform_name: 'OpenAI',
      today_cost_usd: 146.2964,
      today_request_count: 1602,
      today_token_count: 172500000,
    },
    {
      platform_name: 'Claude',
      today_cost_usd: 9.4768,
      today_request_count: 0,
      today_token_count: 0,
    },
  ],
} as const;

export const DASHBOARD_AI_QUOTA_MONITOR_SAMPLE_DATA = {
  service_label: 'Claude Code',
  plan_label: 'Pro',
  status_label: '正常',
  primary_window_label: '5小时限额',
  primary_used_percent: 68,
  primary_reset_at_label: '05-27 20:00',
  secondary_window_label: '周限额',
  secondary_used_percent: 41,
  secondary_reset_at_label: '06-03 08:00',
  updated_label: '05-27 16:30',
} as const;

export const DASHBOARD_SYSTEM_TEMPLATE_SAMPLE_DATA: Record<string, Record<string, unknown>> = {
  ai_usage_stats: DASHBOARD_AI_USAGE_STATS_SAMPLE_DATA,
  ai_quota_monitor: DASHBOARD_AI_QUOTA_MONITOR_SAMPLE_DATA,
};
