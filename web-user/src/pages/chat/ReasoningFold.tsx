import { useState } from 'react';

type Props = {
  itemId: string;
  text: string;
  status: 'streaming' | 'completed';
};

export function ReasoningFold({ text, status }: Props) {
  const [open, setOpen] = useState(status === 'streaming');
  const title = status === 'streaming' ? '思考中…' : '已思考';
  const hasBody = text.trim().length > 0;
  return (
    <div
      className="mb-2 rounded-md border border-zinc-200 bg-zinc-50 text-sm text-zinc-600"
      data-testid="reasoning-fold"
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
        <pre className="border-t border-zinc-200 px-3 py-2 whitespace-pre-wrap text-xs">
          {text}
        </pre>
      ) : null}
      {open && !hasBody && status === 'completed' ? (
        <p className="border-t border-zinc-200 px-3 py-2 text-xs text-zinc-400">
          思考内容未保留在本次加载中
        </p>
      ) : null}
    </div>
  );
}
