import { isAudioDynamicConfig, type DynamicConfig, type DynamicType } from '@/api';
import { ErrorBoundary } from '@/components/feedback/ErrorBoundary';
import { FormSection } from '@/components/ui/FormSection';
import { Input } from '@/components/ui/Input';
import { DashboardConfigPanel } from '@/features/dynamic/components/config/DashboardConfig';
import { DynamicAudioSection } from './config/DynamicAudioSection';
import { DynamicRefreshSettings } from './config/RefreshSettings';
import { FontTestConfigPanel } from './config/FontTestConfig';
import { HistoryTodayConfigPanel } from './config/HistoryTodayConfig';
import { HotListConfigPanel } from './config/HotListConfig';
import { WeatherAlertConfigPanel } from './config/WeatherAlertConfig';
import { WeatherConfigPanel } from './config/WeatherConfig';

export function DynamicContentFields({
  type,
  config,
  frameName,
  onFrameNameChange,
  onConfigChange,
  showParams,
  showAudio,
  contentId,
  dashboardData,
  onDashboardDataChange,
  dashboardDataLabel,
}: {
  type: DynamicType;
  config: DynamicConfig;
  frameName: string;
  onFrameNameChange: (value: string) => void;
  onConfigChange: (config: DynamicConfig) => void;
  showParams: boolean;
  showAudio: boolean;
  contentId?: string;
  dashboardData?: Record<string, unknown> | null;
  onDashboardDataChange?: (data: Record<string, unknown>) => void;
  dashboardDataLabel?: string;
}) {
  return (
    <>
      {type === 'dashboard' && (
        <FormSection label="帧名称（选填，最多 64 字）">
          <Input
            type="text"
            maxLength={64}
            value={frameName}
            onChange={(event) => onFrameNameChange(event.target.value)}
            placeholder="如：AI 限额监控"
          />
        </FormSection>
      )}

      {showParams && (
        <FormSection label="类型参数">
          <ErrorBoundary
            resetKey={`${config.type}:${contentId ?? ''}`}
            fallback={
              <p className="font-sans text-[12px] leading-relaxed text-clay">
                配置加载异常，请返回后重试。
              </p>
            }
          >
            <DynamicConfigFields
              config={config}
              onChange={onConfigChange}
              contentId={contentId}
              dashboardData={dashboardData ?? undefined}
              onDashboardDataChange={type === 'dashboard' ? onDashboardDataChange : undefined}
              dashboardDataLabel={dashboardDataLabel}
            />
          </ErrorBoundary>
        </FormSection>
      )}

      {showAudio && isAudioDynamicConfig(config) && (
        <FormSection label="音频">
          <DynamicAudioSection config={config} onChange={onConfigChange} />
        </FormSection>
      )}
    </>
  );
}

function DynamicConfigFields({
  config,
  onChange,
  contentId,
  dashboardData,
  onDashboardDataChange,
  dashboardDataLabel,
}: {
  config: DynamicConfig;
  onChange: (config: DynamicConfig) => void;
  contentId?: string;
  dashboardData?: Record<string, unknown>;
  onDashboardDataChange?: (data: Record<string, unknown>) => void;
  dashboardDataLabel?: string;
}) {
  switch (config.type) {
    case 'daily_calendar':
    case 'month_calendar':
      return null;
    case 'weather':
      return <WeatherConfigPanel config={config} onChange={onChange} />;
    case 'history_today':
      return <HistoryTodayConfigPanel config={config} onChange={onChange} />;
    case 'weather_alert':
      return <WeatherAlertConfigPanel config={config} onChange={onChange} />;
    case 'earthquake_report':
      return <DynamicRefreshSettings config={config} onChange={onChange} />;
    case 'dashboard':
      return (
        <DashboardConfigPanel
          config={config}
          onChange={onChange}
          contentId={contentId}
          dashboardData={dashboardData ?? {}}
          onDashboardDataChange={onDashboardDataChange}
          dataLabel={dashboardDataLabel}
        />
      );
    case 'font_test':
      return <FontTestConfigPanel config={config} onChange={onChange} />;
    case 'hot_list':
      return <HotListConfigPanel config={config} onChange={onChange} />;
  }
}
