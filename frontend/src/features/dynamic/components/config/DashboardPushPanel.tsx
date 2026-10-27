import { useEffect, useMemo, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { fieldBaseCls } from '@/components/ui/styles/form';
import { cn } from '@/lib/cn';

export function DashboardPushPanel({
  contentId,
  data,
}: {
  contentId: string;
  data: Record<string, unknown>;
}) {
  const [copied, setCopied] = useState(false);
  const copiedTimerRef = useRef<number | null>(null);
  const url = useMemo(() => absoluteContentDataUrl(contentId), [contentId]);
  const examplePayload = useMemo(() => {
    return { version: 1, data };
  }, [data]);
  const exampleCurl = useMemo(
    () =>
      `curl -X POST -H 'Content-Type: application/json' --data-binary @- \\\n  ${url} <<'JSON'\n${JSON.stringify(examplePayload, null, 2)}\nJSON`,
    [examplePayload, url]
  );

  function copy() {
    void navigator.clipboard.writeText(url);
    setCopied(true);
    if (copiedTimerRef.current !== null) window.clearTimeout(copiedTimerRef.current);
    copiedTimerRef.current = window.setTimeout(() => {
      copiedTimerRef.current = null;
      setCopied(false);
    }, 1500);
  }

  useEffect(() => {
    return () => {
      if (copiedTimerRef.current !== null) window.clearTimeout(copiedTimerRef.current);
    };
  }, []);

  return (
    <div className="space-y-3">
      <p className="font-mono text-[10px] text-stone uppercase tracking-[0.18em]">数据推送 URL</p>
      <div className="flex gap-2 items-start">
        <code
          className={cn(
            fieldBaseCls,
            'block min-w-0 flex-1 py-1.5 font-mono text-[11px] break-all'
          )}
        >
          {url}
        </code>
        <button
          type="button"
          onClick={copy}
          className="px-2 py-1.5 text-stone hover:text-ink hover:bg-cream border border-ink flex-shrink-0"
          title="复制"
        >
          {copied ? <Check size={14} /> : <Copy size={14} />}
        </button>
      </div>
      <details className="text-[11px]">
        <summary className="font-mono text-stone uppercase tracking-[0.12em] cursor-pointer">
          示例 curl
        </summary>
        <pre className="mt-2 p-3 bg-cream border border-line text-[10px] leading-snug overflow-x-auto whitespace-pre-wrap">
          {exampleCurl}
        </pre>
      </details>
      <p className="font-sans text-[11px] text-stone italic leading-snug">
        推送流程：① 复制 URL → ② 由你的系统/脚本 POST 数据 → ③ 设备下次唤醒时拉取并刷新屏幕。 URL
        中的 contentId 即推送凭证（随机 ID 不可枚举），请勿公开分享，泄漏后只能删内容重建。
        推送后不会立即亮屏，设备按预设周期或按键翻页时生效。
      </p>
    </div>
  );
}

function absoluteContentDataUrl(contentId: string): string {
  const path = `/api/contents/${contentId}/data`;
  if (typeof window === 'undefined') return path;
  return new URL(path, window.location.origin).toString();
}
