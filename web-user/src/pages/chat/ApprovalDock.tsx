import { useEffect, useEffectEvent, useId, useRef, useState } from 'react';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { decideApproval, type RememberScope } from '@/services/approval';
import { errorMessage } from '@/services/http';
import {
  approvalCopy,
  approvalCountdown,
  type PendingApproval,
} from './approval-copy';

const actions = [
  { key: '1', label: '允许本次', decision: 'approve', remember: 'once' },
  {
    key: '2',
    label: '本对话不再询问',
    decision: 'approve',
    remember: 'conversation',
  },
  {
    key: '3',
    label: '以后在这个 Agent 上都允许',
    decision: 'approve',
    remember: 'agent',
  },
  { key: '4', label: '拒绝', decision: 'reject', remember: 'once' },
] as const;
const agentDescription =
  '将更新该 Agent 的工具设置，只影响之后的对话，可在右侧工具设置改回。';
type Action = { decision: 'approve' | 'reject'; remember: RememberScope };

/** 当前审批随会话挂在输入区，决定提交后由服务端事件收起。 */
export function ApprovalDock({
  part,
  onWarning,
}: {
  part: PendingApproval;
  onWarning: (message: string) => void;
}) {
  const { approval } = part;
  const { title, action, sentence, subject, detailItems } = approvalCopy(part);
  const descriptionId = useId();
  const detailsId = useId();
  const dock = useRef<HTMLElement>(null);
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const submitting = useRef(false);
  const [busy, setBusy] = useState(false);
  const [details, setDetails] = useState(false);
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const [now, setNow] = useState(Date.now);
  const countdown = approvalCountdown(approval.expires_at, now);
  const options = actions.filter(
    (item) => item.remember !== 'agent' || approval.can_remember_agent,
  );

  const submit = async ({ decision, remember }: Action, note?: string) => {
    if (submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError('');
    try {
      const result = await decideApproval(approval.approval_id, {
        decision,
        remember,
        reason: note,
      });
      if (result.warning) onWarning(result.warning);
      if (remember === 'agent' && !result.warning)
        window.dispatchEvent(new Event('xa:tools-updated'));
    } catch (cause) {
      submitting.current = false;
      setBusy(false);
      setError(errorMessage(cause));
    }
  };

  const onKeyDown = useEffectEvent((event: KeyboardEvent) => {
    if (
      submitting.current ||
      event.defaultPrevented ||
      event.repeat ||
      event.isComposing ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey
    )
      return;
    const target = event.target;
    if (
      !(target instanceof HTMLElement) ||
      target.closest(
        'input, textarea, select, [contenteditable], [role="dialog"], [data-approval-details]',
      )
    )
      return;
    const index = options.findIndex(
      (item) => buttons.current.get(item.key) === target,
    );
    if (index >= 0 && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      const offset = event.key === 'ArrowDown' ? 1 : -1;
      buttons.current
        .get(options[(index + offset + options.length) % options.length].key)
        ?.focus();
      event.preventDefault();
      return;
    }
    if (target !== document.body && !dock.current?.contains(target)) return;
    const selected = options.find((item) => item.key === event.key);
    if (!selected) return;
    event.preventDefault();
    void submit(selected);
  });

  useEffect(() => {
    const primary = buttons.current.get('1');
    if (primary) {
      primary.dataset.initialFocus = 'true';
      primary.focus({ preventScroll: true });
    }
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener('keydown', onKeyDown);
    };
  }, []);

  return (
    <section
      ref={dock}
      aria-label={`需要确认${title}`}
      aria-busy={busy}
      className="approval-dock"
      data-testid="approval-dock"
    >
      <p aria-live="polite" className="sr-only">
        需要确认{title}，{sentence}
      </p>
      <div className="mb-1 flex items-center justify-between gap-3 text-xs text-zinc-400">
        <span>{title}</span>
        <button
          type="button"
          data-approval-details
          aria-expanded={details}
          aria-controls={detailsId}
          disabled={busy}
          onClick={() => setDetails(!details)}
        >
          详情
        </button>
      </div>
      <p className="approval-sentence" data-testid="approval-sentence">
        {subject ? (
          <>
            要忘掉「<strong className="font-semibold">{subject}</strong>
            」这条记忆。
          </>
        ) : (
          sentence
        )}
      </p>
      <div id={detailsId} data-approval-details hidden={!details}>
        {details && (
          <div className="mb-3 space-y-2 text-xs text-zinc-500">
            {detailItems.length > 0 && (
              <dl>
                {detailItems.map((item) => (
                  <div
                    key={`${item.label}-${item.value}`}
                    className="flex gap-2"
                  >
                    <dt className="shrink-0">{item.label}</dt>
                    <dd className="min-w-0 break-words">{item.value}</dd>
                  </div>
                ))}
              </dl>
            )}
            {part.args_preview && (
              <pre
                className="max-h-32 overflow-auto whitespace-pre-wrap break-all"
                data-testid="approval-arguments"
              >
                {part.args_preview}
              </pre>
            )}
            <details>
              <summary className="cursor-pointer">附说明后拒绝</summary>
              <div className="mt-1 flex items-center gap-2">
                <input
                  className="min-w-0 flex-1 border-b bg-transparent py-1"
                  aria-label="拒绝说明"
                  value={reason}
                  maxLength={200}
                  disabled={busy}
                  onChange={(event) => setReason(event.target.value)}
                />
                <button
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    void submit(
                      { decision: 'reject', remember: 'once' },
                      reason,
                    )
                  }
                >
                  拒绝
                </button>
              </div>
            </details>
          </div>
        )}
      </div>
      <div className="approval-options">
        {options.map((item) => {
          const button = (
            <button
              key={item.key}
              type="button"
              className="approval-option"
              data-primary={item.key === '1' || undefined}
              ref={(node) => {
                if (node) buttons.current.set(item.key, node);
                else buttons.current.delete(item.key);
              }}
              disabled={busy}
              aria-keyshortcuts={item.key}
              aria-label={`${item.label}，${action}`}
              aria-describedby={
                item.remember === 'agent' ? descriptionId : undefined
              }
              onBlur={(event) => {
                delete event.currentTarget.dataset.initialFocus;
              }}
              onClick={() => void submit(item)}
            >
              <span className="approval-option-number" aria-hidden="true">
                {item.key}
              </span>
              {item.label}
            </button>
          );
          return item.remember === 'agent' ? (
            <Tooltip key={item.key}>
              <TooltipTrigger asChild>{button}</TooltipTrigger>
              <TooltipContent className="max-w-64">
                {agentDescription}
              </TooltipContent>
            </Tooltip>
          ) : (
            <span key={item.key} className="contents">
              {button}
            </span>
          );
        })}
        <span id={descriptionId} className="sr-only">
          {agentDescription}
        </span>
      </div>
      {countdown && (
        <p
          className="mt-2 text-xs text-zinc-400"
          data-testid="approval-countdown"
        >
          {countdown}
        </p>
      )}
      {error && (
        <p role="alert" className="mt-2 text-xs text-rose-600">
          {error}
        </p>
      )}
    </section>
  );
}
