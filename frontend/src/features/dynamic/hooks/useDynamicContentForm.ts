import { useCallback, useMemo, useState } from 'react';
import type { DynamicConfig, DynamicType } from '@/api';
import { defaultConfig } from '@/features/dynamic/model/default-config';
import {
  defaultDynamicFrameName,
  effectiveDynamicFrameName,
  effectiveDynamicStatusBarText,
  frameNameForSyncedDynamicConfigChange,
} from '@/features/dynamic/model/display-name';
import { dynamicConfigKey } from '@/features/dynamic/model/json-parse';
import { useDynamicPreview } from './useDynamicPreview';

interface UseDynamicContentFormOptions {
  contentId?: string;
  initialType?: DynamicType | null;
  initialConfig?: DynamicConfig | null;
  initialFrameName?: string | null;
  initialDashboardData?: Record<string, unknown> | null;
  requireDashboardData?: boolean;
}

export function useDynamicContentForm({
  contentId,
  initialType = null,
  initialConfig = null,
  initialFrameName = null,
  initialDashboardData = null,
  requireDashboardData = false,
}: UseDynamicContentFormOptions = {}) {
  const [type, setType] = useState<DynamicType | null>(initialType);
  const [config, setConfig] = useState<DynamicConfig | null>(initialConfig);
  const [frameName, setFrameName] = useState(() =>
    initialType && initialConfig
      ? (initialFrameName ?? defaultDynamicFrameName(initialType, initialConfig))
      : ''
  );
  const [dashboardData, setDashboardData] = useState<Record<string, unknown> | null>(
    initialDashboardData
  );
  const { livePreviewData, previewPending, invalidatePreview } = useDynamicPreview({
    contentId,
    type,
    config,
    dashboardData,
  });
  const configKey = useMemo(() => (config ? dynamicConfigKey(config) : ''), [config]);
  const hasDashboardData = dashboardData !== null && Object.keys(dashboardData).length > 0;
  const dashboardDataSatisfied = !requireDashboardData || hasDashboardData;

  const loadType = useCallback(
    (nextType: DynamicType, nextDashboardData: Record<string, unknown> | null = null) => {
      invalidatePreview();
      const nextConfig = defaultConfig(nextType);
      setType(nextType);
      setConfig(nextConfig);
      setFrameName(defaultDynamicFrameName(nextType, nextConfig));
      setDashboardData(nextDashboardData);
    },
    [invalidatePreview]
  );

  const reset = useCallback(() => {
    invalidatePreview();
    setType(null);
    setConfig(null);
    setFrameName('');
    setDashboardData(null);
  }, [invalidatePreview]);

  const changeConfig = useCallback((next: DynamicConfig) => {
    setConfig((previous) => {
      if (previous) {
        const nextFrameName = frameNameForSyncedDynamicConfigChange(previous, next);
        if (nextFrameName) setFrameName(nextFrameName);
      }
      return next;
    });
  }, []);

  const submitConfig = useCallback(() => {
    if (!type || !config) return { ok: false as const, error: 'config: 请选择动态类型' };
    // Field validation happens on the server; it reports the first invalid field.
    if (config.type === 'dashboard' && !dashboardDataSatisfied) {
      return { ok: false as const, error: 'dashboard 初始数据不能为空' };
    }
    return {
      ok: true as const,
      type: config.type,
      config,
      frameName: effectiveDynamicFrameName(config.type, config, frameName),
      dashboardData: config.type === 'dashboard' ? (dashboardData ?? undefined) : undefined,
    };
  }, [config, dashboardData, dashboardDataSatisfied, frameName, type]);

  return {
    type,
    setType,
    config,
    setConfig,
    configKey,
    frameName,
    setFrameName,
    dashboardData,
    setDashboardData,
    hasDashboardData,
    canSubmit: !!(type && config && (type !== 'dashboard' || dashboardDataSatisfied)),
    previewPending,
    livePreviewData,
    caption: effectiveDynamicStatusBarText(type, config, frameName),
    invalidatePreview,
    loadType,
    reset,
    changeConfig,
    submitConfig,
  };
}
