import type { DynamicConfig } from '@/api';
import { CitySearch } from './CitySearch';
import { DynamicRefreshSettings } from './RefreshSettings';
import type { DynamicConfigChange } from '@/features/dynamic/model/config-types';

export function WeatherConfigPanel({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: DynamicConfigChange;
}) {
  return (
    <div className="space-y-4">
      <CitySearch
        value={config.locationLabel ?? ''}
        onSelect={({ locationId, label }) =>
          onChange({
            ...config,
            locationId: locationId,
            locationLabel: label,
          })
        }
      />
      <DynamicRefreshSettings config={config} onChange={onChange} />
    </div>
  );
}
