import { useLayoutEffect, useRef } from 'react';
import { Icon } from '@/components/Icon';

type Props = {
  value: string;
  disabled: boolean;
  sending: boolean;
  creating: boolean;
  onChange: (value: string) => void;
  onSend: () => void;
  onStop: () => void;
  onCreate: () => void;
};

/** 输入区只管理草稿交互，发送状态由会话状态层提供。 */
export function MessageComposer({
  value,
  disabled,
  sending,
  creating,
  onChange,
  onSend,
  onStop,
  onCreate,
}: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);
  // 草稿改变后按真实内容测量，最多占用六行左右的高度。
  // biome-ignore lint/correctness/useExhaustiveDependencies: value 变化时重新计算 textarea 的高度。
  useLayoutEffect(() => {
    const input = ref.current;
    if (!input) return;
    input.style.height = 'auto';
    input.style.height = `${Math.min(input.scrollHeight, 160)}px`;
  }, [value]);
  return (
    <div className="mx-auto w-full max-w-[760px] shrink-0 px-4 pb-4">
      <div className="message-composer flex items-end gap-2 rounded-[28px] border border-zinc-200 bg-white px-2 py-2 shadow-[0_8px_30px_rgba(24,24,27,0.08)]">
        <button
          type="button"
          className="mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100"
          aria-label="添加"
          title="新建 Agent"
          onClick={onCreate}
        >
          <Icon name="plus" size={18} />
        </button>
        <textarea
          ref={ref}
          aria-label="消息"
          className="max-h-40 min-h-9 flex-1 resize-none border-0 bg-transparent px-1 py-1.5 text-[15px] leading-6 outline-none"
          value={value}
          disabled={disabled}
          readOnly={creating}
          placeholder={disabled ? '暂不可发送' : '发消息'}
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
              if (!sending && !disabled && value.trim()) onSend();
            }
          }}
        />
        {sending && !creating ? (
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
  );
}
