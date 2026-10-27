import type { DynamicConfig } from '@/api';
import { ProvinceSearch } from './ProvinceSearch';
import { DynamicRefreshSettings } from './RefreshSettings';
import type { DynamicConfigChange } from '@/features/dynamic/model/config-types';

export function WeatherAlertConfigPanel({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: DynamicConfigChange;
}) {
  return (
    <div className="space-y-4">
      <ProvinceSearch
        value={config.province ?? ''}
        onSelect={(province) => onChange({ ...config, province })}
      />
      <DynamicRefreshSettings config={config} onChange={onChange} />
    </div>
  );
}
