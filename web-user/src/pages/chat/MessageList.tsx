import { Link } from '@umijs/max';
import { useLayoutEffect, useRef, useState } from 'react';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
import type { Agent } from '@/services/api';
import type { ChatMessage } from '@/services/chat';

type Props = {
  agent?: Agent;
  messages: ChatMessage[];
  loading: boolean;
  error: string;
  onRetry: () => void;
  onLoadOlder: () => Promise<void>;
  hasMore: boolean;
  loadingOlder: boolean;
  olderError: string;
};

/** 消息只在底部跟随；向前分页用首条可见消息作为锚点保持阅读位置。 */
export function MessageList({
  agent,
  messages,
  loading,
  error,
  onRetry,
  onLoadOlder,
  hasMore,
  loadingOlder,
  olderError,
}: Props) {
  const viewport = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const anchor = useRef<{ id: string; offset: number } | null>(null);
  const [showLatest, setShowLatest] = useState(false);
  const scrollToBottom = () => {
    if (viewport.current)
      viewport.current.scrollTop = viewport.current.scrollHeight;
    follow.current = true;
    setShowLatest(false);
  };
  useLayoutEffect(() => {
    const node = content.current;
    if (!node) return;
    const observer = new ResizeObserver(() => {
      if (follow.current && viewport.current)
        viewport.current.scrollTop = viewport.current.scrollHeight;
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  // biome-ignore lint/correctness/useExhaustiveDependencies: 消息分页提交后恢复同一个 DOM 锚点的位置。
  useLayoutEffect(() => {
    const saved = anchor.current;
    const node = viewport.current;
    if (!saved || !node || loadingOlder) return;
    const element = Array.from(
      node.querySelectorAll<HTMLElement>('[data-message-id]'),
    ).find((item) => item.dataset.messageId === saved.id);
    if (element)
      node.scrollTop += element.getBoundingClientRect().top - saved.offset;
    anchor.current = null;
  }, [messages, loadingOlder]);
  const older = () => {
    const first =
      viewport.current?.querySelector<HTMLElement>('[data-message-id]');
    if (!hasMore || loadingOlder || !first) return;
    follow.current = false;
    anchor.current = {
      id: first.dataset.messageId || '',
      offset: first.getBoundingClientRect().top,
    };
    void onLoadOlder();
  };
  return (
    <div className="message-viewport-wrap">
      <div
        className="conversation-body"
        ref={viewport}
        onScroll={() => {
          const node = viewport.current;
          if (!node) return;
          follow.current =
            node.scrollHeight - node.scrollTop - node.clientHeight < 80;
          setShowLatest(!follow.current);
          if (node.scrollTop < 60 && !loading && !olderError) older();
        }}
      >
        <div className="message-content" ref={content}>
          {loading ? (
            <div className="message-list-state" role="status">
              正在读取对话…
            </div>
          ) : error ? (
            <div className="message-list-state error" role="alert">
              <p>{error}</p>
              <button type="button" className="button" onClick={onRetry}>
                重新加载
              </button>
            </div>
          ) : !messages.length ? (
            <div className="conversation-empty">
              <h1>{agent?.name || '选择 Agent'}</h1>
              {!agent && (
                <Link to="/explore" className="text-button">
                  发现 Agent
                </Link>
              )}
            </div>
          ) : (
            <>
              {hasMore && (
                <div className="history-loader">
                  {olderError && <span role="alert">{olderError}</span>}
                  <button
                    type="button"
                    className="text-button"
                    disabled={loadingOlder}
                    onClick={older}
                  >
                    {loadingOlder ? '正在读取…' : '更早消息'}
                  </button>
                </div>
              )}
              <div
                className="message-list"
                role="log"
                aria-label="对话消息"
                aria-live="polite"
                aria-relevant="additions"
              >
                {messages.map((message) => (
                  <article
                    key={message.id}
                    data-message-id={message.id}
                    className={`message-row ${message.role === 'user' ? 'user' : 'assistant'}`}
                    aria-label={
                      message.role === 'user'
                        ? '你的消息'
                        : `${agent?.name || '助手'}的回复`
                    }
                  >
                    <div className="message-bubble">
                      {message.content &&
                        (message.role === 'user' ? (
                          <div className="user-message-text">
                            {message.content}
                          </div>
                        ) : (
                          <Markdown>{message.content}</Markdown>
                        ))}
                      {message.status === 'pending' && !message.content && (
                        <span
                          className="typing-indicator"
                          role="status"
                          aria-label="正在生成"
                        >
                          <i />
                          <i />
                          <i />
                        </span>
                      )}
                      {message.status === 'pending' && message.content && (
                        <span className="streaming-caret" aria-hidden="true" />
                      )}
                      {message.status === 'stopped' && (
                        <span className="message-status">已停止生成</span>
                      )}
                      {message.status === 'failed' && (
                        <span className="message-status">
                          {message.error === '已停止生成'
                            ? message.error
                            : '回复未完成，请重新加载记录确认结果。'}
                        </span>
                      )}
                    </div>
                  </article>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
      {showLatest && (
        <button
          type="button"
          className="scroll-latest"
          aria-label="回到最新消息"
          title="回到最新消息"
          onClick={scrollToBottom}
        >
          <Icon name="arrow-down" size={18} />
        </button>
      )}
    </div>
  );
}
