import { useCallback, useRef, useState } from 'react';
import type { Content } from '@/api';
import { DEFAULT_DITHER_MODE, type DitherMode } from '@/lib/eink/dither';
import { useAudioFormState } from './useAudioFormState';
import { useCropState } from './useCropState';
import { useImageFormSubmit } from './useImageFormSubmit';

/** Default threshold for the threshold dither mode. */
const BW_THRESHOLD_DEFAULT = 128;

export function useImageContentForm(content?: Content) {
  const isEdit = !!content;
  const previewRef = useRef<HTMLCanvasElement>(null);
  const [imageFile, setImageFile] = useState<File | null>(null);
  const audio = useAudioFormState(content);
  const [threshold, setThreshold] = useState(BW_THRESHOLD_DEFAULT);
  const [mode, setMode] = useState<DitherMode>(DEFAULT_DITHER_MODE);
  const [frameName, setFrameName] = useState(content?.frameName ?? '');
  const { scale, setScale, offset, setOffset, resetCrop } = useCropState();
  const frameNameChanged = isEdit && frameName !== (content.frameName ?? '');
  const trimmedTtsText = audio.ttsText.trim();
  const existingTtsText = content?.audio?.text?.trim() ?? '';
  const hasExistingTts = isEdit && content?.audio?.source === 'tts';
  const wantsTts =
    audio.audioMode === 'tts' &&
    trimmedTtsText.length > 0 &&
    (!hasExistingTts ||
      content?.audio?.status === 'failed' ||
      trimmedTtsText !== existingTtsText ||
      audio.ttsVoice !== content?.audio?.voice);
  const hasFilePatch = !!imageFile || !!audio.audioFile;
  const hasContentPatch = hasFilePatch || frameNameChanged;
  const canCreate = !!imageFile && (audio.audioMode !== 'tts' || trimmedTtsText.length > 0);
  const canEdit =
    (hasContentPatch || wantsTts) && (audio.audioMode !== 'tts' || trimmedTtsText.length > 0);

  const onImagePick = useCallback(
    (file: File | null) => {
      setImageFile(file);
      resetCrop();
    },
    [resetCrop]
  );

  const reset = useCallback(() => {
    setImageFile(null);
    audio.resetAudio();
    setThreshold(BW_THRESHOLD_DEFAULT);
    setMode(DEFAULT_DITHER_MODE);
    setFrameName('');
    resetCrop();
  }, [audio, resetCrop]);

  const buildFormData = useImageFormSubmit({
    imageFile,
    audioFile: audio.audioFile,
    frameName,
    threshold,
    mode,
    scale,
    offset,
  });

  return {
    image: {
      previewRef,
      file: imageFile,
      setFile: setImageFile,
      onPick: onImagePick,
    },
    audio: {
      file: audio.audioFile,
      setFile: audio.setAudioFile,
      mode: audio.audioMode,
      setMode: audio.setAudioMode,
      ttsText: audio.ttsText,
      setTtsText: audio.setTtsText,
      ttsVoice: audio.ttsVoice,
      setTtsVoice: audio.setTtsVoice,
      trimmedTtsText,
      wantsTts,
    },
    dither: {
      threshold,
      setThreshold,
      mode,
      setMode,
    },
    crop: {
      scale,
      setScale,
      offset,
      setOffset,
      reset: resetCrop,
    },
    frameName,
    setFrameName,
    frameNameChanged,
    hasContentPatch,
    hasFilePatch,
    canCreate,
    canEdit,
    reset,
    buildFormData,
  };
}
