import { useState } from 'react';

type Props = {
  itemId: string;
  text: string;
  status: 'streaming' | 'completed';
};

export function ReasoningFold({ text, status }: Props) {
  const [open, setOpen] = useState(false);
  const title = status === 'streaming' ? '思考中…' : '已思考';
  return (
    <div className="mb-2 rounded-md border border-zinc-200 bg-zinc-50 text-sm text-zinc-600">
      <button
        type="button"
        className="flex w-full items-center justify-between px-3 py-2 text-left"
        onClick={() => setOpen((v) => !v)}
      >
        <span>{title}</span>
        <span className="text-xs text-zinc-400">{open ? '收起' : '展开'}</span>
      </button>
      {open ? (
        <pre className="border-t border-zinc-200 px-3 py-2 whitespace-pre-wrap text-xs">
          {text}
        </pre>
      ) : null}
    </div>
  );
}
