import { Wand2, Upload } from 'lucide-react';
import type { AudioStatus } from '@/api';
import { SegmentToggle } from '@/components/ui/SegmentToggle';
import { AudioDropzone } from './AudioDropzone';
import { TtsFields } from './TtsFields';

export type ImageAudioMode = 'upload' | 'tts';

interface ImageAudioBlockProps {
  gid: string;
  mode: ImageAudioMode;
  onModeChange: (m: ImageAudioMode) => void;
  audioFile: File | null;
  onAudioFileChange: (f: File | null) => void;
  ttsText: string;
  onTtsTextChange: (t: string) => void;
  ttsVoice: string;
  onTtsVoiceChange: (v: string) => void;
  hasExistingAudio: boolean;
  editingContentId: string | null;
  audioStatus?: AudioStatus | null;
  audioError?: string | null;
}

export function ImageAudioBlock({
  gid,
  mode,
  onModeChange,
  audioFile,
  onAudioFileChange,
  ttsText,
  onTtsTextChange,
  ttsVoice,
  onTtsVoiceChange,
  hasExistingAudio,
  editingContentId,
  audioStatus,
  audioError,
}: ImageAudioBlockProps) {
  return (
    <div className="space-y-4">
      <SegmentToggle<ImageAudioMode>
        value={mode}
        onChange={(m) => {
          onModeChange(m);
          if (m === 'tts') onAudioFileChange(null);
        }}
        options={[
          {
            value: 'upload',
            label: (
              <>
                <Upload size={12} />
                上传
              </>
            ),
          },
          {
            value: 'tts',
            label: (
              <>
                <Wand2 size={12} />
                TTS
              </>
            ),
          },
        ]}
      />
      {mode === 'upload' ? (
        <AudioDropzone
          gid={gid}
          hasExistingAudio={hasExistingAudio}
          editingContentId={editingContentId}
          audioFile={audioFile}
          onPick={onAudioFileChange}
          hideLabel
        />
      ) : (
        <TtsFields
          text={ttsText}
          onTextChange={onTtsTextChange}
          voice={ttsVoice}
          onVoiceChange={onTtsVoiceChange}
          status={audioStatus}
          error={audioError}
        />
      )}
    </div>
  );
}
