import { useEffect, useId, useState } from 'react';
import { errorMessage } from '@/services/http';
import { getToolResult, type ToolResult } from '@/services/tool-results';

/** 展开时才请求完整结果；折叠或离开消息时终止请求，不缓存其他轮次的数据。 */
export function ToolResultDetails({
  runId,
  callId,
}: {
  runId: string;
  callId: string;
}) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [revision, setRevision] = useState(0);
  const [result, setResult] = useState<ToolResult>();
  const [error, setError] = useState('');
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 用于显式重试。
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setResult(undefined);
    setError('');
    getToolResult(runId, callId, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setResult(value);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [runId, callId, open, revision]);
  return (
    <div>
      <button
        type="button"
        className="py-1 text-xs text-zinc-500 underline underline-offset-4 hover:text-zinc-800"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((value) => !value)}
      >
        {open ? '收起详情' : '查看详情'}
      </button>
      {open && (
        <section id={id} className="space-y-2 py-1" aria-label="工具结果详情">
          {error ? (
            <p role="alert" className="text-rose-600">
              {error}
              <button
                type="button"
                className="ml-2 underline underline-offset-4"
                onClick={() => setRevision((value) => value + 1)}
              >
                重试
              </button>
            </p>
          ) : !result ? (
            <p role="status">正在读取…</p>
          ) : (
            <>
              <p className="text-zinc-400">输入</p>
              <pre className="max-h-64 max-w-full overflow-auto whitespace-pre-wrap break-all font-mono text-[11px]">
                {JSON.stringify(result.arguments, null, 2)}
              </pre>
              <p className="text-zinc-400">结果</p>
              {result.data == null ? (
                <p>没有保留完整结果</p>
              ) : (
                <pre className="max-h-64 max-w-full overflow-auto whitespace-pre-wrap break-all font-mono text-[11px]">
                  {JSON.stringify(result.data, null, 2)}
                </pre>
              )}
            </>
          )}
        </section>
      )}
    </div>
  );
}
