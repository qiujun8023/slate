import { catalog, fontInfo, type DynamicConfig } from '@/api';
import { Select, SelectItem } from '@/components/ui/Select';
import { Checkbox } from '@/components/ui/Checkbox';
import type { DynamicConfigChange } from '@/features/dynamic/model/config-types';

export function FontTestConfigPanel({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: DynamicConfigChange;
}) {
  const selectedFont = fontInfo(config.fontId);

  return (
    <div className="space-y-4">
      <div>
        <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">字体</p>
        <Select
          value={config.fontId ?? 'fusion_pixel_12'}
          onValueChange={(value) => {
            onChange({ ...config, fontId: value });
          }}
        >
          {catalog.fonts.map((option) => (
            <SelectItem key={option.id} value={option.id} hint={option.hint}>
              {option.label}
            </SelectItem>
          ))}
        </Select>
      </div>
      {selectedFont && (
        <div className="space-y-1.5">
          <p className="font-sans text-[12px] text-stone leading-relaxed italic">
            {selectedFont.note}
          </p>
          <p className="mt-1.5 font-mono text-[10px] text-stone truncate">
            {selectedFont.source} · {selectedFont.license}
          </p>
        </div>
      )}
      <Checkbox
        label="反白测试"
        checked={config.invert ?? false}
        onChange={(value) => onChange({ ...config, invert: value })}
      />
    </div>
  );
}
