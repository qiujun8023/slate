import { TTS_VOICES, isTtsVoice, type AudioStatus } from '@/api';
import { Select, SelectItem } from '@/components/ui/Select';
import { inputCls } from '@/components/ui/styles/form';
import { cn } from '@/lib/cn';

interface TtsFieldsProps {
  text: string;
  onTextChange: (text: string) => void;
  voice: string;
  onVoiceChange: (voice: string) => void;
  status?: AudioStatus | null;
  error?: string | null;
}

export function TtsFields({
  text,
  onTextChange,
  voice,
  onVoiceChange,
  status,
  error,
}: TtsFieldsProps) {
  return (
    <div className="space-y-3">
      <label className="block">
        <span className="block font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">
          TTS 文案
        </span>
        <textarea
          value={text}
          onChange={(e) => onTextChange(e.target.value.slice(0, 500))}
          className={cn(inputCls, 'min-h-28 resize-y font-sans text-[14px] leading-6')}
          placeholder="输入这张图片要播放的语音文案"
        />
      </label>
      <div>
        <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em] mb-1.5">音色</p>
        <Select
          value={voice}
          onValueChange={(value) => {
            if (!isTtsVoice(value)) return;
            onVoiceChange(value);
          }}
        >
          {TTS_VOICES.map((item) => (
            <SelectItem key={item} value={item}>
              {item}
            </SelectItem>
          ))}
        </Select>
      </div>
      {status === 'generating' || status === 'pending' ? (
        <p className="font-sans text-[11px] text-stone">音频生成中</p>
      ) : status === 'failed' && error ? (
        <p className="font-sans text-[11px] text-clay truncate" title={error}>
          {error}
        </p>
      ) : null}
    </div>
  );
}
