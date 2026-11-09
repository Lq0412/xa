import { useEffect, useState } from 'react';

type Props = {
  itemId: string;
  text: string;
  status: 'streaming' | 'completed';
  /** 本轮助手消息仍在生成时，不把思考块显示成历史占位态。 */
  liveRun?: boolean;
};

export function ReasoningFold({ text, status, liveRun }: Props) {
  const effective = liveRun ? 'streaming' : status;
  const [open, setOpen] = useState(effective === 'streaming');
  const title = effective === 'streaming' ? '思考中…' : '已思考';
  const hasBody = text.trim().length > 0;
  useEffect(() => {
    if (effective === 'streaming' || (liveRun && hasBody)) setOpen(true);
  }, [effective, liveRun, hasBody]);
  return (
    <div
      className="mb-2 rounded-md border border-zinc-200 bg-zinc-50 text-sm text-zinc-600"
      data-testid="reasoning-fold"
      data-reasoning-status={effective}
      data-live-run={liveRun ? '1' : '0'}
    >
      <button
        type="button"
        className="flex w-full items-center justify-between px-3 py-2 text-left"
        onClick={() => setOpen((v) => !v)}
      >
        <span>{title}</span>
        <span className="text-xs text-zinc-400">{open ? '收起' : '展开'}</span>
      </button>
      {open && hasBody ? (
        <pre
          className="border-t border-zinc-200 px-3 py-2 whitespace-pre-wrap text-xs"
          data-testid="reasoning-fold-body"
        >
          {text}
        </pre>
      ) : null}
      {open && !hasBody && !liveRun && status === 'completed' ? (
        <p className="border-t border-zinc-200 px-3 py-2 text-xs text-zinc-400">
          思考内容未保留在本次加载中
        </p>
      ) : null}
    </div>
  );
}
