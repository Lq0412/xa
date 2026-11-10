import { useLayoutEffect, useRef, useState } from 'react';
import { Icon } from '@/components/Icon';
import { ApprovalDock } from './ApprovalDock';
import type { PendingApproval } from './approval-copy';

type Props = {
  value: string;
  disabled: boolean;
  sending: boolean;
  creating: boolean;
  approval?: PendingApproval;
  onChange: (value: string) => void;
  onSend: () => void;
  onStop: () => void;
};

/** 输入区只管理草稿交互，发送状态由会话状态层提供。 */
export function MessageComposer({
  value,
  disabled,
  sending,
  creating,
  approval,
  onChange,
  onSend,
  onStop,
}: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [warning, setWarning] = useState('');
  // 草稿改变后按真实内容测量，最多占用六行左右的高度。
  // biome-ignore lint/correctness/useExhaustiveDependencies: value 变化时重新计算 textarea 的高度。
  useLayoutEffect(() => {
    const input = ref.current;
    if (!input) return;
    input.style.height = 'auto';
    input.style.height = `${Math.min(input.scrollHeight, 160)}px`;
  }, [value]);
  return (
    <div className="composer-wrap">
      {warning && (
        <div className="mb-2 flex items-center justify-between gap-2 px-2 text-xs text-amber-700">
          <p role="status">{warning}</p>
          <button
            type="button"
            aria-label="关闭提示"
            onClick={() => setWarning('')}
          >
            ×
          </button>
        </div>
      )}
      <div className="composer-stack" data-testid="composer-stack">
        {approval && (
          <ApprovalDock
            key={approval.approval.approval_id}
            part={approval}
            onWarning={setWarning}
          />
        )}
        <div className="message-composer flex shrink-0 items-end gap-2 px-3 py-2">
          <textarea
            ref={ref}
            aria-label="消息"
            className="max-h-40 min-h-9 min-w-0 flex-1 resize-none border-0 bg-transparent px-1 py-1.5 text-[15px] leading-6 outline-none placeholder:text-zinc-400"
            value={value}
            disabled={disabled || Boolean(approval)}
            readOnly={creating}
            placeholder={
              approval
                ? '请先确认上方的操作'
                : disabled
                  ? '暂不可发送'
                  : '发消息'
            }
            rows={1}
            onChange={(event) => onChange(event.target.value)}
            onKeyDown={(event) => {
              if (
                event.key === 'Enter' &&
                !event.shiftKey &&
                !event.nativeEvent.isComposing &&
                event.keyCode !== 229
              ) {
                event.preventDefault();
                if (!sending && !disabled && !approval && value.trim())
                  onSend();
              }
            }}
          />
          {approval || (sending && !creating) ? (
            <button
              type="button"
              className="mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full bg-[#f5c518] text-[#3a2a00]"
              aria-label="停止生成"
              onClick={onStop}
            >
              <Icon name="stop" size={14} />
            </button>
          ) : (
            <button
              type="button"
              className="mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full bg-[#f5c518] text-[#3a2a00] disabled:opacity-40"
              aria-label="发送消息"
              title="发送（Enter）· 换行（Shift + Enter）"
              disabled={disabled || sending || !value.trim()}
              onClick={onSend}
            >
              <Icon name="arrow-up" size={16} />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
