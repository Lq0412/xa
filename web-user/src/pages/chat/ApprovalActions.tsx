import { useEffect, useId, useRef, useState } from 'react';
import { decideApproval, type RememberScope } from '@/services/approval';
import type { ToolPart } from '@/services/chat-parts';
import { errorMessage } from '@/services/http';

/** 确认操作保持在过程时间线里，提交后等待服务器事件更新状态。 */
export function ApprovalActions({ part }: { part: ToolPart }) {
  const approval = part.approval;
  const button = useRef<HTMLButtonElement>(null);
  const descriptionId = useId();
  const [remember, setRemember] = useState<RememberScope>('once');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [warning, setWarning] = useState('');
  const [now, setNow] = useState(Date.now());
  const waiting = part.status === 'awaiting_approval';
  useEffect(() => {
    if (!waiting) return;
    button.current?.focus({ preventScroll: true });
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [waiting]);
  if (!approval) return null;
  if (!waiting)
    return warning ? (
      <p role="status" className="pl-6 text-xs">
        {warning}
      </p>
    ) : null;
  const title = part.title || part.name;
  const preview =
    approval.items?.find((item) => item.label === '内容')?.value ||
    approval.summary;
  const minutes = Math.max(
    0,
    Math.ceil((Date.parse(approval.expires_at) - now) / 60000),
  );
  const decide = async (decision: 'approve' | 'reject') => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const result = await decideApproval(approval.approval_id, {
        decision,
        remember: decision === 'approve' ? remember : 'once',
        reason,
      });
      if (result.warning) setWarning(result.warning);
      if (decision === 'approve' && remember === 'agent')
        window.dispatchEvent(new Event('xa:tools-updated'));
    } catch (cause) {
      setError(errorMessage(cause));
      setBusy(false);
    }
  };
  return (
    <div
      className="space-y-2 pb-2 pl-6 text-xs"
      data-testid="approval-actions"
      aria-busy={busy}
    >
      <p role="status" aria-live="polite" className="sr-only">
        需要确认{title}
      </p>
      {approval.summary && <p>{approval.summary}</p>}
      {approval.items?.map((item) => (
        <p key={item.label}>
          {item.label}：{item.value}
        </p>
      ))}
      {!approval.items?.length && part.args_preview && (
        <pre className="whitespace-pre-wrap break-all">{part.args_preview}</pre>
      )}
      <p className="text-zinc-400">
        {minutes > 0 ? `剩余 ${minutes} 分钟` : '等待超时处理'}
      </p>
      <div className="flex flex-col gap-1 sm:flex-row sm:gap-4">
        <button
          ref={button}
          type="button"
          className="min-h-11 rounded-full bg-[#f5c518] px-4 text-[#3a2a00] disabled:opacity-50"
          disabled={busy}
          aria-label={`允许本次${title}${preview ? `，${preview}` : ''}`}
          onClick={() => void decide('approve')}
        >
          {busy ? '正在提交…' : '允许本次'}
        </button>
        <button
          type="button"
          className="min-h-11 px-2 text-left disabled:opacity-50"
          disabled={busy}
          aria-label={`拒绝${title}${preview ? `，${preview}` : ''}`}
          onClick={() => void decide('reject')}
        >
          拒绝
        </button>
      </div>
      <label className="flex min-h-8 items-center gap-2">
        <input
          type="checkbox"
          disabled={busy}
          checked={remember === 'conversation'}
          onChange={(event) =>
            setRemember(event.target.checked ? 'conversation' : 'once')
          }
        />
        本对话不再询问「{title}」
      </label>
      {approval.can_remember_agent && (
        <>
          <label className="flex min-h-8 items-center gap-2">
            <input
              type="checkbox"
              disabled={busy}
              checked={remember === 'agent'}
              aria-describedby={descriptionId}
              onChange={(event) =>
                setRemember(event.target.checked ? 'agent' : 'once')
              }
            />
            以后在这个 Agent 上都允许
          </label>
          <p
            id={descriptionId}
            className={remember === 'agent' ? 'text-zinc-400' : 'sr-only'}
          >
            将更新该 Agent 的工具设置，只影响后续对话，可在右侧工具设置改回。
          </p>
        </>
      )}
      <details>
        <summary className="cursor-pointer py-1">补充说明</summary>
        <input
          className="w-full border-b bg-transparent py-2 outline-none"
          aria-label="审批补充说明"
          maxLength={200}
          value={reason}
          disabled={busy}
          onChange={(event) => setReason(event.target.value)}
        />
      </details>
      {error && (
        <p role="alert" className="text-rose-600">
          {error}
        </p>
      )}
    </div>
  );
}
