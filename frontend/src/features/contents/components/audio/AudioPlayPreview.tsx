// 音频播放预览。server 给的是 raw PCM，播放控制由 usePcmAudioPlayer 封装。

import { useState, useCallback, useEffect } from 'react';
import { Loader2, Pause, Play } from 'lucide-react';
import { useBlob } from '@/features/contents/query/content-image-queries';
import { useToast } from '@/components/feedback/toast-context';
import { cn } from '@/lib/cn';
import { usePcmAudioPlayer } from './usePcmAudioPlayer';

interface AudioPlayPreviewProps {
  /** 音频 blob 的 sha256；为空表示无音频，组件返 null */
  hash: string | null;
  className?: string;
  /** 标签：显示在按钮旁（可选，如「试听」） */
  label?: string;
}

export function AudioPlayPreview({ hash, className, label }: AudioPlayPreviewProps) {
  const [requested, setRequested] = useState(false);
  const [playAfterLoad, setPlayAfterLoad] = useState(false);
  const audio = useBlob(requested ? hash : null);
  const toast = useToast();
  const onPlaybackError = useCallback(
    (message: string, hint?: string) => toast.error(message, hint),
    [toast]
  );
  const { playing, ensureContext, play, stop } = usePcmAudioPlayer({
    onError: onPlaybackError,
  });

  useEffect(() => {
    if (!playAfterLoad || !audio.data) return;
    setPlayAfterLoad(false);
    void play(audio.data);
  }, [audio.data, play, playAfterLoad]);

  useEffect(() => {
    if (audio.error) setPlayAfterLoad(false);
  }, [audio.error]);

  useEffect(() => {
    stop();
    setRequested(false);
    setPlayAfterLoad(false);
  }, [hash, stop]);

  if (!hash) return null;

  const loading = requested && audio.isFetching;
  const title = audio.error ? '音频加载失败' : loading ? '加载中' : playing ? '停止' : '试听';

  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        e.preventDefault();
        if (loading || audio.error) return;
        if (playing) {
          setPlayAfterLoad(false);
          stop();
          return;
        }
        if (audio.data) {
          void play(audio.data);
        } else {
          setRequested(true);
          setPlayAfterLoad(true);
          void ensureContext()
            .then((ctx) => {
              if (ctx) return;
              setPlayAfterLoad(false);
              toast.error('音频播放失败', '当前环境不支持 WebAudio。');
            })
            .catch((err) => {
              setPlayAfterLoad(false);
              toast.error('音频播放失败', err instanceof Error ? err.message : undefined);
            });
        }
      }}
      disabled={loading || !!audio.error}
      title={title}
      aria-label={title}
      className={cn(
        'inline-flex items-center gap-1.5 px-2 py-1.5 text-stone hover:text-ink hover:bg-cream transition-colors disabled:opacity-50',
        className
      )}
    >
      {loading ? (
        <Loader2 size={14} className="animate-spin" />
      ) : playing ? (
        <Pause size={14} />
      ) : (
        <Play size={14} />
      )}
      {label && <span className="font-sans text-[12px]">{label}</span>}
    </button>
  );
}
