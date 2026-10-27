import { Select, SelectItem } from '@/components/ui/Select';
import type { DynamicConfig } from '@/api';
import type { DynamicConfigChange } from '@/features/dynamic/model/config-types';

const DEFAULT_REFRESH_INTERVAL_SEC = 600;

export function DynamicRefreshSettings({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: DynamicConfigChange;
}) {
  const current = config.refreshIntervalSec ?? DEFAULT_REFRESH_INTERVAL_SEC;
  return (
    <div>
      <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">
        刷新间隔
      </p>
      <Select
        value={String(current)}
        onValueChange={(value) => onChange({ ...config, refreshIntervalSec: Number(value) })}
      >
        {refreshOptions(config.type).map((item) => (
          <SelectItem key={item.value} value={String(item.value)} hint={item.hint}>
            {item.label}
          </SelectItem>
        ))}
      </Select>
    </div>
  );
}

function refreshOptions(type?: string): Array<{
  value: number;
  label: string;
  hint: string;
}> {
  return [
    ...(type === 'dashboard' ? [{ value: 60, label: '1 分钟', hint: '高频' }] : []),
    { value: 300, label: '5 分钟', hint: '更实时' },
    { value: 600, label: '10 分钟', hint: '推荐' },
    { value: 1800, label: '30 分钟', hint: '省电' },
    { value: 3600, label: '1 小时', hint: '低频' },
  ];
}
