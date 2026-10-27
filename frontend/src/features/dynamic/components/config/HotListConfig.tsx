import { catalog, type DynamicConfig, type HotListSourceInfo } from '@/api';
import { Select, SelectItem } from '@/components/ui/Select';
import { DynamicRefreshSettings } from './RefreshSettings';
import type { DynamicConfigChange } from '@/features/dynamic/model/config-types';

const collator = new Intl.Collator('zh-Hans-CN-u-co-pinyin', {
  numeric: true,
  sensitivity: 'base',
});

function displayLabel(source: HotListSourceInfo): string {
  return source.channelLabel ? `${source.label} · ${source.channelLabel}` : source.label;
}

const SOURCES_BY_NAME = [...catalog.hotListSources].sort((a, b) =>
  collator.compare(displayLabel(a), displayLabel(b))
);

export function HotListConfigPanel({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: DynamicConfigChange;
}) {
  return (
    <div className="space-y-4">
      <div>
        <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">频道</p>
        <Select
          value={config.source ?? 'weibo'}
          onValueChange={(value) => {
            onChange({ ...config, source: value });
          }}
        >
          {SOURCES_BY_NAME.map((source) => (
            <SelectItem key={source.id} value={source.id} hint={hotListKindLabel(source.kind)}>
              {displayLabel(source)}
            </SelectItem>
          ))}
        </Select>
      </div>
      <DynamicRefreshSettings config={config} onChange={onChange} />
    </div>
  );
}

function hotListKindLabel(kind: HotListSourceInfo['kind']): string {
  switch (kind) {
    case 'general':
      return '综合';
    case 'news':
      return '新闻';
    case 'tech':
      return '科技';
    case 'community':
      return '社区';
    case 'commerce':
      return '消费';
  }
}
