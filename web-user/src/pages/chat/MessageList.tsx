import { Link } from '@umijs/max';
import { useLayoutEffect, useRef, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
import { Skeleton } from '@/components/ui/skeleton';
import type { Agent } from '@/services/api';
import type { ChatMessage } from '@/services/chat';
import type { MessagePart } from '@/services/chat-parts';
import {
  reorderReasoningBeforeText,
  splitAssistantParts,
} from './assistant-parts';
import { ProcessRail } from './ProcessRail';

type Props = {
  agent?: Agent;
  messages: ChatMessage[];
  loading: boolean;
  error: string;
  canSuggest: boolean;
  onSuggest: (text: string) => void;
  onRetry: () => void;
  onLoadOlder: () => Promise<void>;
  hasMore: boolean;
  loadingOlder: boolean;
  olderError: string;
};

const suggestions = ['介绍一下你自己', '你能帮我做什么', '我们从哪里开始'];

function partKey(part: MessagePart) {
  if (part.type === 'reasoning') return `reasoning-${part.item_id}`;
  if (part.type === 'tool')
    return `tool-${part.call_id ?? part.name}-${part.index ?? 0}`;
  return `text-${part.text.length}-${part.text.slice(0, 12)}`;
}

function pad(value: number) {
  return String(value).padStart(2, '0');
}

function sameDay(left: Date, right: Date) {
  return (
    left.getFullYear() === right.getFullYear() &&
    left.getMonth() === right.getMonth() &&
    left.getDate() === right.getDate()
  );
}

/** 同一分钟只显示一次，格式与参考界面一致。 */
function timeLabel(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const now = new Date();
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  const clock = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  if (sameDay(date, now)) return `今天 ${clock}`;
  if (sameDay(date, yesterday)) return `昨天 ${clock}`;
  return `${date.getMonth() + 1}月${date.getDate()}日 ${clock}`;
}

/** 消息只在底部跟随；向前分页用首条可见消息作为锚点保持阅读位置。 */
export function MessageList({
  agent,
  messages,
  loading,
  error,
  canSuggest,
  onSuggest,
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
  const stamps = useRef(new Map<string, string>());
  const [showLatest, setShowLatest] = useState(false);
  const stamp = (message: ChatMessage) => {
    if (message.created_at) return message.created_at;
    const known = stamps.current.get(message.id);
    if (known) return known;
    const next = new Date().toISOString();
    stamps.current.set(message.id, next);
    return next;
  };
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
    <div className="relative flex min-h-0 flex-1">
      <div
        className="conversation-body min-h-0 w-full overflow-auto overscroll-contain px-6 pt-2 pb-2 [overflow-anchor:none]"
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
        <div className="flex min-h-full flex-col" ref={content}>
          {loading ? (
            <div
              className="m-auto w-full max-w-[760px] space-y-4 py-8"
              role="status"
            >
              <span className="sr-only">正在读取对话</span>
              <Skeleton className="h-16 w-[58%]" />
              <Skeleton className="ml-auto h-12 w-[36%] bg-[#fff1c2]" />
              <Skeleton className="h-24 w-[64%]" />
            </div>
          ) : error ? (
            <div
              className="m-auto max-w-md px-6 py-8 text-center text-sm text-rose-700"
              role="alert"
            >
              <p>{error}</p>
              <button
                type="button"
                className="mt-4 rounded-xl border border-zinc-200 bg-white px-3 py-2 text-zinc-800"
                onClick={onRetry}
              >
                重新加载
              </button>
            </div>
          ) : !messages.length ? (
            <div className="m-auto flex max-w-md flex-col items-center px-6 py-10 text-center">
              {agent ? (
                <>
                  <AgentAvatar size={96} />
                  <h1 className="mt-4 text-2xl font-medium text-zinc-900">
                    {agent.name}
                  </h1>
                  <p className="mt-2 line-clamp-1 text-sm text-zinc-500">
                    {agent.description || '发一条消息，开始这段对话。'}
                  </p>
                  {canSuggest && (
                    <div className="mt-6 flex flex-col items-center gap-2">
                      {suggestions.map((question) => (
                        <button
                          key={question}
                          type="button"
                          className="rounded-full border border-zinc-200 bg-white px-4 py-2 text-sm text-zinc-700 hover:bg-zinc-50"
                          onClick={() => onSuggest(question)}
                        >
                          {question}
                        </button>
                      ))}
                    </div>
                  )}
                </>
              ) : (
                <>
                  <h1 className="text-2xl font-medium text-zinc-900">
                    选择 Agent
                  </h1>
                  <Link to="/explore" className="mt-4 text-sm underline">
                    发现 Agent
                  </Link>
                </>
              )}
            </div>
          ) : (
            <>
              {hasMore && (
                <div className="py-2 text-center text-xs text-zinc-400">
                  {olderError && (
                    <span className="mb-1 block text-rose-700" role="alert">
                      {olderError}
                    </span>
                  )}
                  <button
                    type="button"
                    className="underline"
                    disabled={loadingOlder}
                    onClick={older}
                  >
                    {loadingOlder ? '正在读取…' : '更早消息'}
                  </button>
                </div>
              )}
              <div
                className="mx-auto flex w-full max-w-[760px] flex-col gap-5 py-4"
                role="log"
                aria-label="对话消息"
                aria-live="polite"
                aria-relevant="additions"
              >
                {messages.map((message, index) => {
                  const label = timeLabel(stamp(message));
                  const previous =
                    index > 0 ? timeLabel(stamp(messages[index - 1])) : '';
                  const mine = message.role === 'user';
                  const orderedParts = message.parts?.length
                    ? reorderReasoningBeforeText(message.parts)
                    : [];
                  const { process: processParts, text: textParts } =
                    splitAssistantParts(orderedParts);
                  const hasTextPart = textParts.length > 0;
                  const showContentFallback =
                    !hasTextPart && Boolean(message.content?.trim()) && !mine;
                  const showBubble =
                    mine ||
                    hasTextPart ||
                    showContentFallback ||
                    message.status === 'stopped' ||
                    message.status === 'failed';
                  return (
                    <div key={message.id} className="flex flex-col gap-3">
                      {label && label !== previous && (
                        <p className="text-center text-xs text-zinc-400">
                          {label}
                        </p>
                      )}
                      <article
                        data-message-id={message.id}
                        data-message-status={message.status}
                        data-parts-count={message.parts?.length ?? 0}
                        className={`message-row flex w-full min-w-0 ${mine ? 'user justify-end' : 'assistant justify-start'}`}
                        aria-label={
                          mine ? '你的消息' : `${agent?.name || '助手'}的回复`
                        }
                      >
                        {mine ? (
                          <div className="user-bubble max-w-[70%] rounded-2xl bg-[#fff4cc] px-4 py-2.5 text-[15px] leading-7 text-zinc-900">
                            {message.content ? (
                              <div className="whitespace-pre-wrap">
                                {message.content}
                              </div>
                            ) : null}
                          </div>
                        ) : (
                          <div className="flex w-full max-w-[760px] flex-col gap-2">
                            {processParts.length > 0 ? (
                              <ProcessRail
                                parts={processParts}
                                messageStatus={message.status}
                              />
                            ) : null}
                            {showBubble ? (
                              <div className="assistant-bubble max-w-[70%] rounded-2xl bg-zinc-100 px-4 py-2.5 text-[15px] leading-7 text-zinc-900">
                                {hasTextPart
                                  ? textParts.map((part) => (
                                      <div
                                        key={partKey(part)}
                                        data-testid="message-text-part"
                                      >
                                        <Markdown>{part.text}</Markdown>
                                      </div>
                                    ))
                                  : null}
                                {showContentFallback ? (
                                  <div data-testid="message-text-part">
                                    <Markdown>{message.content}</Markdown>
                                  </div>
                                ) : null}
                                {message.status === 'pending' ? (
                                  <>
                                    {!hasTextPart && !showContentFallback ? (
                                      <span className="sr-only">正在生成</span>
                                    ) : null}
                                    <span
                                      className="streaming-caret"
                                      aria-hidden="true"
                                    />
                                  </>
                                ) : null}
                                {message.status === 'stopped' ? (
                                  <span className="mt-2 block text-xs text-amber-700">
                                    已停止生成
                                  </span>
                                ) : null}
                                {message.status === 'failed' ? (
                                  <span className="mt-2 block text-xs text-amber-700">
                                    {message.error === '已停止生成'
                                      ? message.error
                                      : '回复未完成，请重新加载记录确认结果。'}
                                  </span>
                                ) : null}
                              </div>
                            ) : null}
                          </div>
                        )}
                      </article>
                    </div>
                  );
                })}
              </div>
            </>
          )}
        </div>
      </div>
      {showLatest && (
        <button
          type="button"
          className="absolute bottom-3 left-1/2 flex size-9 -translate-x-1/2 items-center justify-center rounded-full border border-zinc-200 bg-white shadow-sm"
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
