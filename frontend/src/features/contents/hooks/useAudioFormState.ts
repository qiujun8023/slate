import { useCallback, useMemo, useState } from 'react';
import { DEFAULT_TTS_VOICE, type Content } from '@/api';
import type { ImageAudioMode } from '@/features/contents/components/image-form/ImageAudioBlock';

export function useAudioFormState(content?: Content) {
  const [audioFile, setAudioFile] = useState<File | null>(null);
  const [audioMode, setAudioMode] = useState<ImageAudioMode>(
    content?.audio?.source === 'tts' ? 'tts' : 'upload'
  );
  const [ttsText, setTtsText] = useState(content?.audio?.text ?? '');
  const [ttsVoice, setTtsVoice] = useState<string>(content?.audio?.voice ?? DEFAULT_TTS_VOICE);

  const resetAudio = useCallback(() => {
    setAudioFile(null);
    setAudioMode('upload');
    setTtsText('');
    setTtsVoice(DEFAULT_TTS_VOICE);
  }, []);

  return useMemo(
    () => ({
      audioFile,
      setAudioFile,
      audioMode,
      setAudioMode,
      ttsText,
      setTtsText,
      ttsVoice,
      setTtsVoice,
      resetAudio,
    }),
    [audioFile, audioMode, resetAudio, ttsText, ttsVoice]
  );
}
