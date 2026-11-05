import { ArrowUpOutlined, StopOutlined } from '@ant-design/icons';
import { useLayoutEffect, useRef } from 'react';

type Props = {
  value: string;
  disabled: boolean;
  sending: boolean;
  creating: boolean;
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
  onChange,
  onSend,
  onStop,
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
    <div className="composer-area">
      <div className="message-composer">
        <textarea
          ref={ref}
          aria-label="消息"
          value={value}
          disabled={disabled}
          readOnly={creating}
          placeholder={disabled ? '暂不可发送' : '发送消息'}
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
        <div className="composer-footer">
          {sending && !creating ? (
            <button
              type="button"
              className="send-button stop"
              aria-label="停止生成"
              onClick={onStop}
            >
              <StopOutlined />
            </button>
          ) : (
            <button
              type="button"
              className="send-button"
              aria-label="发送消息"
              title="发送（Enter）· 换行（Shift + Enter）"
              disabled={disabled || sending || !value.trim()}
              onClick={onSend}
            >
              <ArrowUpOutlined />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
