import type { DynamicConfig } from '@/api';
import { Select, SelectItem } from '@/components/ui/Select';
import type { DynamicConfigChange } from '@/features/dynamic/model/config-types';

export function HistoryTodayConfigPanel({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: DynamicConfigChange;
}) {
  return (
    <div>
      <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">数据源</p>
      <Select
        value={config.source ?? 'wikipedia'}
        onValueChange={(value) => {
          onChange({ ...config, source: value });
        }}
      >
        <SelectItem value="wikipedia" hint="默认">
          维基百科
        </SelectItem>
        <SelectItem value="baidu_baike" hint="百科">
          百度百科
        </SelectItem>
      </Select>
    </div>
  );
}
