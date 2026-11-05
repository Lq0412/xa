import {
  ArrowDownOutlined,
  RobotOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Link } from '@umijs/max';
import { Avatar } from 'antd';
import { useLayoutEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Agent } from '@/services/api';
import type { ChatMessage } from '@/services/chat';

type Props = {
  agent?: Agent;
  messages: ChatMessage[];
  loading: boolean;
  error: string;
  onRetry: () => void;
};

/** 消息区保持单一纵向流，Markdown 只渲染服务端文本，不注入原始 HTML。 */
export function MessageList({
  agent,
  messages,
  loading,
  error,
  onRetry,
}: Props) {
  const viewport = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const [showLatest, setShowLatest] = useState(false);
  const scrollToBottom = () => {
    if (viewport.current)
      viewport.current.scrollTop = viewport.current.scrollHeight;
    follow.current = true;
    setShowLatest(false);
  };
  // ResizeObserver 只在用户停留底部时跟随，阅读历史时保留滚动位置。
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
              <button type="button" onClick={onRetry}>
                重新加载
              </button>
            </div>
          ) : !messages.length ? (
            <div className="conversation-empty">
              <Avatar
                size={56}
                src={agent?.avatar || undefined}
                icon={<RobotOutlined />}
              />
              <h1>{agent?.name || '选择 Agent'}</h1>
              {!agent && (
                <Link className="chat-explore-link" to="/explore">
                  发现 Agent
                </Link>
              )}
            </div>
          ) : (
            <div
              className="message-list"
              role="log"
              aria-label="对话消息"
              aria-live="polite"
              aria-relevant="additions"
            >
              {messages.map((message) => {
                const user = message.role === 'user';
                return (
                  <article
                    key={message.id}
                    className={`message-row ${user ? 'user' : 'assistant'}`}
                    aria-label={
                      user ? '你的消息' : `${agent?.name || '助手'}的回复`
                    }
                  >
                    <Avatar
                      size={32}
                      src={user ? undefined : agent?.avatar || undefined}
                      icon={user ? <UserOutlined /> : <RobotOutlined />}
                    />
                    <div className="message-content-wrap">
                      <div
                        className={`message-bubble ${message.status === 'failed' ? 'failed' : ''}`}
                      >
                        {message.content ? (
                          user ? (
                            <div className="user-message-text">
                              {message.content}
                            </div>
                          ) : (
                            <ReactMarkdown
                              remarkPlugins={[remarkGfm]}
                              skipHtml
                              components={{
                                a: ({ href, children }) => (
                                  <a
                                    href={href}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                  >
                                    {children}
                                  </a>
                                ),
                              }}
                            >
                              {message.content}
                            </ReactMarkdown>
                          )
                        ) : null}
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
                          <span
                            className="streaming-caret"
                            aria-hidden="true"
                          />
                        )}
                        {message.status === 'stopped' && (
                          <span className="message-status">已停止生成</span>
                        )}
                        {message.status === 'failed' && (
                          <span className="message-status">
                            {message.error === '已停止生成'
                              ? '已停止生成'
                              : '回复未完成，请重新加载记录确认结果。'}
                          </span>
                        )}
                      </div>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </div>
      </div>
      {showLatest && (
        <button
          type="button"
          className="scroll-latest"
          onClick={scrollToBottom}
        >
          <ArrowDownOutlined /> 回到最新消息
        </button>
      )}
    </div>
  );
}
