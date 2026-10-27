import { Radio } from 'lucide-react';
import type { Group } from '@/api';
import { Select, SelectItem, SelectSeparator } from '@/components/ui/Select';

export function DeviceGroupSelector({
  groups,
  value,
  onChange,
  disabled,
}: {
  groups: Group[];
  value: string | null;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  return (
    <section>
      <div className="flex items-center gap-2 mb-2 text-stone">
        <Radio size={14} />
        <h3 className="font-sans text-[12px] uppercase tracking-wide">在播</h3>
      </div>
      <GroupSelector groups={groups} value={value} onChange={onChange} disabled={disabled} />
      <p className="font-serif text-[12px] italic text-stone-light mt-2">
        切换后会立即向设备入队同步动作。
      </p>
    </section>
  );
}

function GroupSelector({
  groups,
  value,
  onChange,
  disabled,
}: {
  groups: Group[];
  value: string | null;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const showNone = value === null;
  return (
    <Select
      value={value ?? '__none__'}
      onValueChange={onChange}
      disabled={disabled}
      placeholder="未选组"
      aria-label="切换在播组"
    >
      {showNone && <SelectItem value="__none__">未选组</SelectItem>}
      {showNone && groups.length > 0 && <SelectSeparator />}
      {groups.map((group) => (
        <SelectItem key={group.id} value={group.id} hint={`${group.contentCount} 项`}>
          <span className="font-serif">{group.name}</span>
        </SelectItem>
      ))}
    </Select>
  );
}
