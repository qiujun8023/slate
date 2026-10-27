import { DEFAULT_TTS_VOICE, TTS_VOICES, isTtsVoice, type DynamicConfig } from '@/api';
import { Select, SelectItem } from '@/components/ui/Select';
import { Checkbox } from '@/components/ui/Checkbox';

export function DynamicAudioSection({
  config,
  onChange,
}: {
  config: DynamicConfig;
  onChange: (config: DynamicConfig) => void;
}) {
  return (
    <div className="space-y-3">
      <Checkbox
        label="生成音频"
        checked={config.audioEnabled ?? false}
        onChange={(value) => onChange({ ...config, audioEnabled: value })}
      />
      <div>
        <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">音色</p>
        <Select
          value={config.audioVoice ?? DEFAULT_TTS_VOICE}
          onValueChange={(value) => {
            if (!isTtsVoice(value)) return;
            onChange({ ...config, audioVoice: value });
          }}
          disabled={!config.audioEnabled}
        >
          {TTS_VOICES.map((voice) => (
            <SelectItem key={voice} value={voice}>
              {voice}
            </SelectItem>
          ))}
        </Select>
      </div>
    </div>
  );
}
