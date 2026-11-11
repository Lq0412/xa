import { history } from '@umijs/max';
import { useCallback, useEffect, useRef, useState } from 'react';
import { type Agent, getAgent } from '@/services/api';
import {
  type ActiveRun,
  type ChatMessage,
  type Conversation,
  cancelRun,
  chatURL,
  createConversation,
  listConversations,
  listMessages,
  mergeMessagePartsFromLocal,
  resumeRun,
  type StreamRunResult,
  streamRun,
} from '@/services/chat';
import { applyRunEvent } from '@/services/chat-run';
import { errorMessage } from '@/services/http';
import type { MemoryCue } from './MemoryPanel';
import { pendingApprovals } from './process-summary';

/** 每个会话独立保存草稿和请求状态，流式回调始终按会话 ID 更新。 */
type Session = {
  agent?: Agent;
  messages: ChatMessage[];
  draft: string;
  loaded: boolean;
  loading: boolean;
  sending: boolean;
  creating: boolean;
  loadError: string;
  sendError: string;
  hasMore: boolean;
  loadingOlder: boolean;
  before?: string;
  olderError: string;
};
const emptySession: Session = {
  messages: [],
  draft: '',
  loaded: false,
  loading: false,
  sending: false,
  creating: false,
  loadError: '',
  sendError: '',
  hasMore: false,
  loadingOlder: false,
  olderError: '',
};

type ActiveTurn = {
  controller: AbortController;
  assistantId: string;
  runId?: string;
  stopRequested?: boolean;
  finished?: boolean;
};

/** 停止时立即收起仍在进行的思考和工具，保留已有正文及执行结果。 */
function stopMessage(message: ChatMessage): ChatMessage {
  return {
    ...message,
    status: 'stopped',
    parts: message.parts?.map((part) => {
      if (part.type === 'reasoning' && part.status === 'streaming')
        return { ...part, status: 'stopped' };
      if (
        part.type === 'tool' &&
        (part.status === 'preparing' ||
          part.status === 'pending' ||
          part.status === 'running' ||
          part.status === 'awaiting_approval')
      )
        return {
          ...part,
          status: 'cancelled',
          approval: part.approval
            ? { ...part.approval, decision: 'cancelled' }
            : undefined,
        };
      return part;
    }),
  };
}

/** 最新一页替换对应的本地消息，已加载的旧页和本轮思考正文继续保留。 */
function mergeRunHistory(
  state: Session,
  page: Awaited<ReturnType<typeof listMessages>>,
  localUserId?: string,
): Partial<Session> {
  const latest = page.messages.map((message) => {
    const local = state.messages.find(
      (item) =>
        item.id === message.id ||
        (message.role === 'assistant' &&
          message.run_id &&
          item.run_id === message.run_id &&
          item.role === 'assistant'),
    );
    if (message.parts?.length)
      return mergeMessagePartsFromLocal(message, local);
    if (local?.parts?.length)
      return {
        ...message,
        parts: local.parts,
        content: message.content || local.content,
      };
    return message;
  });
  const ids = new Set(latest.map((message) => message.id));
  const savedRuns = new Set(
    latest
      .filter((message) => message.role === 'assistant' && message.run_id)
      .map((message) => message.run_id),
  );
  const keepLocal = (message: ChatMessage) =>
    !ids.has(message.id) &&
    message.id !== localUserId &&
    !(
      message.role === 'assistant' &&
      message.run_id &&
      savedRuns.has(message.run_id)
    );
  const firstLatest = state.messages.findIndex((message) =>
    ids.has(message.id),
  );
  const split = firstLatest < 0 ? state.messages.length : firstLatest;
  const older = state.messages.slice(0, split).filter(keepLocal);
  // 进行中的本地回复还没有历史记录，重试最新页时保留它原来的末尾位置。
  const local = state.messages.slice(split).filter(keepLocal);
  const keepCursor = firstLatest > 0 && older.length > 0;
  return {
    messages: [...older, ...latest, ...local],
    hasMore: keepCursor ? state.hasMore : page.has_more,
    before: keepCursor ? state.before : page.before,
  };
}

/** 状态层协调真实历史和生成过程，组件只消费当前会话的快照。 */
export function useChatSession(search: string, defaultAgent?: Agent) {
  const params = new URLSearchParams(search);
  const agentID = params.get('agentId') || params.get('agent');
  const conversationID =
    params.get('conversationId') || params.get('conversation');
  const key = conversationID || `new:${agentID || ''}`;
  const selectedKey = useRef(key);
  selectedKey.current = key;
  const [sessions, setSessions] = useState(new Map<string, Session>());
  const cache = useRef(sessions);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState('');
  const [revision, setRevision] = useState(0);
  const [memoryCue, setMemoryCue] = useState<MemoryCue>({
    agentId: '',
    seq: 0,
  });
  const controllers = useRef(new Map<string, AbortController>());
  const turns = useRef(new Map<string, ActiveTurn>());
  const mounted = useRef(true);

  const update = useCallback(
    (
      id: string,
      patch: Partial<Session> | ((current: Session) => Partial<Session>),
    ) => {
      if (!mounted.current) return;
      const current = cache.current.get(id) || emptySession;
      const next = new Map(cache.current);
      next.set(id, {
        ...current,
        ...(typeof patch === 'function' ? patch(current) : patch),
      });
      cache.current = next;
      setSessions(next);
    },
    [],
  );

  const emitMemoryCue = useCallback(
    (agentId: string, reason: NonNullable<MemoryCue['reason']>) => {
      if (!mounted.current) return;
      setMemoryCue((cue) => {
        const seq = cue.seq + 1;
        return {
          ...cue,
          agentId,
          seq,
          reason,
          ...(reason === 'memory_updated'
            ? { memoryUpdateSeq: seq }
            : { turnCompleted: { agentId, seq } }),
        };
      });
    },
    [],
  );

  const cancelTurn = useCallback(
    async (targetKey: string, turn: ActiveTurn) => {
      if (!turn.runId || turn.finished) return;
      try {
        // 202 只表示后端收到取消请求。继续订阅，等终态写入后再开放输入。
        await cancelRun(turn.runId, turn.controller.signal);
      } catch (cause) {
        if (!turn.controller.signal.aborted && !turn.finished) {
          turn.stopRequested = false;
          update(targetKey, { sendError: errorMessage(cause) });
        }
      }
    },
    [update],
  );

  const finishTurn = useCallback(
    async (
      targetKey: string,
      convId: string,
      turn: ActiveTurn,
      result: StreamRunResult,
      agentId: string,
      localUserId?: string,
    ) => {
      turn.finished = true;
      if (result.status === 'completed')
        emitMemoryCue(agentId, 'turn_completed');
      update(targetKey, (state) => ({
        sendError: result.error ?? '',
        messages: state.messages.map((message) => {
          if (message.id !== turn.assistantId) return message;
          const saved = {
            ...message,
            id: result.messageId || message.id,
            run_id: result.runId,
            status: result.status,
            error: result.error,
          };
          return result.status === 'stopped' ? stopMessage(saved) : saved;
        }),
      }));
      const page = await listMessages(convId, turn.controller.signal);
      update(targetKey, (state) => mergeRunHistory(state, page, localUserId));
    },
    [emitMemoryCue, update],
  );

  const resumeActiveRun = useCallback(
    async (
      convId: string,
      targetKey: string,
      active: ActiveRun,
      agentId: string,
    ) => {
      if (active.status !== 'running' && active.status !== 'waiting_approval')
        return;
      if (turns.current.has(targetKey)) return;
      const messages = cache.current.get(targetKey)?.messages ?? [];
      const existing = messages.find(
        (message) =>
          message.role === 'assistant' && message.run_id === active.id,
      );
      const last = messages.at(-1);
      const pending =
        last?.role === 'assistant' && last.status === 'pending' && !last.run_id
          ? last
          : undefined;
      const assistantId = existing?.id ?? pending?.id ?? `resume-${active.id}`;
      const fallbackParts = (active.pending_approvals ?? []).reduce(
        (parts, approval) =>
          applyRunEvent(parts, {
            ...approval,
            type: 'approval.required',
            seq: 0,
          }),
        [] as NonNullable<ChatMessage['parts']>,
      );
      const controller = new AbortController();
      const turn = {
        assistantId,
        controller,
        runId: active.id,
      } satisfies ActiveTurn;
      turns.current.set(targetKey, turn);
      update(targetKey, (state) => ({
        sending: true,
        messages:
          existing || pending
            ? state.messages.map((message) =>
                message.id === assistantId
                  ? {
                      ...message,
                      status: 'pending',
                      run_id: active.id,
                      parts: fallbackParts.length
                        ? fallbackParts
                        : message.parts,
                    }
                  : message,
              )
            : [
                ...state.messages,
                {
                  id: assistantId,
                  conversation_id: convId,
                  role: 'assistant',
                  content: '',
                  status: 'pending',
                  run_id: active.id,
                  parts: fallbackParts,
                },
              ],
      }));
      try {
        const result = await resumeRun(active.id, controller.signal, {
          onMemoryUpdated: () => emitMemoryCue(agentId, 'memory_updated'),
          onParts: (parts, content) => {
            // 服务端快照可先显示按钮，重放事件仍按原始顺序构建时间线。
            const restored = (active.pending_approvals ?? []).reduce(
              (current, approval) => {
                const existing = current.find(
                  (part) =>
                    part.type === 'tool' && part.call_id === approval.call_id,
                );
                if (
                  existing?.type === 'tool' &&
                  (existing.approval || existing.status !== 'pending')
                )
                  return current;
                return applyRunEvent(current, {
                  ...approval,
                  type: 'approval.required',
                  seq: 0,
                });
              },
              parts,
            );
            update(targetKey, (state) => ({
              messages: state.messages.map((message) =>
                message.id === assistantId
                  ? { ...message, content, parts: restored, run_id: active.id }
                  : message,
              ),
            }));
          },
        });
        await finishTurn(targetKey, convId, turn, result, agentId);
      } catch (cause) {
        if (!controller.signal.aborted) {
          const error = errorMessage(cause);
          update(targetKey, (state) => ({
            sendError: error,
            messages: state.messages.map((message) =>
              message.id === assistantId && message.status === 'pending'
                ? { ...message, status: 'failed', error }
                : message,
            ),
          }));
        }
      } finally {
        if (turns.current.get(targetKey) === turn) {
          turns.current.delete(targetKey);
          update(targetKey, { sending: false });
        }
      }
    },
    [emitMemoryCue, finishTurn, update],
  );

  useEffect(() => {
    mounted.current = true;
    const requests = controllers.current;
    const streams = turns.current;
    return () => {
      mounted.current = false;
      for (const request of requests.values()) request.abort();
      requests.clear();
      for (const turn of streams.values()) turn.controller.abort();
      streams.clear();
    };
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 是用户重试列表读取的版本号。
  useEffect(() => {
    const controller = new AbortController();
    setListLoading(true);
    setListError('');
    listConversations(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted)
          setConversations(result.conversations ?? []);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setListError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setListLoading(false);
      });
    return () => controller.abort();
  }, [revision]);

  useEffect(() => {
    if (
      !agentID &&
      !conversationID &&
      !listLoading &&
      !listError &&
      defaultAgent
    ) {
      const recent = conversations.find(
        (item) => item.agent_id === defaultAgent.id,
      );
      history.replace(chatURL(defaultAgent.id, recent?.id));
    }
  }, [
    agentID,
    conversationID,
    listLoading,
    listError,
    conversations,
    defaultAgent,
  ]);

  const load = useCallback(async () => {
    if (listLoading || listError) return;
    if (!conversationID && agentID) {
      const existing = conversations.find((item) => item.agent_id === agentID);
      if (existing) {
        history.replace(chatURL(agentID, existing.id));
        return;
      }
    }
    const current = cache.current.get(key);
    if (current?.loading || current?.sending) return;
    const conversation = conversations.find(
      (item) => item.id === conversationID,
    );
    if (conversationID && !conversation) {
      update(key, { loadError: '会话不存在或当前账号无权访问', loaded: false });
      return;
    }
    const targetAgent = conversation?.agent_id || agentID;
    if (!targetAgent) return;
    const controller = new AbortController();
    controllers.current.set(`load:${key}`, controller);
    // 最新一页会重算游标，上一页的在途请求不能再更新同一会话。
    controllers.current.get(`older:${key}`)?.abort();
    controllers.current.delete(`older:${key}`);
    update(key, { loading: true, loadingOlder: false, loadError: '' });
    try {
      const [agent, result] = await Promise.all([
        getAgent(targetAgent, controller.signal),
        conversationID
          ? listMessages(conversationID, controller.signal)
          : Promise.resolve({
              messages: [],
              has_more: false,
              before: undefined,
              active_run: undefined,
            }),
      ]);
      update(key, (state) => ({
        ...mergeRunHistory(state, result),
        agent,
        olderError: '',
        loaded: true,
        sendError: '',
      }));
      if (conversationID && result.active_run) {
        void resumeActiveRun(
          conversationID,
          key,
          result.active_run,
          targetAgent,
        );
      }
      if (
        selectedKey.current === key &&
        search !== chatURL(targetAgent, conversationID || undefined).slice(5)
      )
        history.replace(chatURL(targetAgent, conversationID || undefined));
    } catch (cause) {
      if (!controller.signal.aborted)
        update(key, { loadError: errorMessage(cause), loaded: false });
    } finally {
      update(key, { loading: false });
      controllers.current.delete(`load:${key}`);
    }
  }, [
    agentID,
    conversationID,
    conversations,
    key,
    listError,
    listLoading,
    search,
    update,
    resumeActiveRun,
  ]);

  useEffect(() => {
    const current = cache.current.get(key);
    if (!current?.loaded && !current?.loadError) void load();
  }, [key, load]);

  const send = async () => {
    const current = cache.current.get(key);
    const text = current?.draft.trim();
    if (
      !current?.loaded ||
      !current.agent ||
      current.agent.status !== 'active' ||
      !text ||
      current.sending ||
      current.loading ||
      current.messages.some(
        (message) =>
          message.status === 'pending' &&
          message.parts?.some(
            (part) =>
              part.type === 'tool' && part.status === 'awaiting_approval',
          ),
      )
    )
      return;
    // 浏览器已知离线时保留草稿，不创建会话或发起运行。
    if (!navigator.onLine) {
      update(key, { sendError: '无法连接服务，请检查网络后重试' });
      return;
    }
    const controller = new AbortController();
    const turnAgentID = current.agent.id;
    let targetKey = key;
    const assistantID = `local-assistant-${crypto.randomUUID()}`;
    const turn: ActiveTurn = { assistantId: assistantID, controller };
    turns.current.set(key, turn);
    // 同步更新缓存，连续按 Enter 也只能发起一次请求。
    update(key, { sending: true, creating: !conversationID, sendError: '' });
    const localUserID = `local-user-${crypto.randomUUID()}`;
    let submitted = false;
    try {
      let id = conversationID;
      if (!id) {
        const created = await createConversation(
          current.agent.id,
          controller.signal,
        );
        if (!mounted.current) return;
        id = created.id;
        const existing = await listMessages(id, controller.signal);
        targetKey = id;
        update(targetKey, {
          ...current,
          messages: existing.messages ?? [],
          hasMore: existing.has_more,
          before: existing.before,
          sending: true,
          draft: '',
          creating: false,
        });
        update(key, { sending: false, creating: false, draft: '' });
        turns.current.delete(key);
        turns.current.set(targetKey, turn);
        setConversations((items) => [
          created,
          ...items.filter((item) => item.id !== created.id),
        ]);
        if (selectedKey.current === key)
          history.replace(chatURL(created.agent_id, id));
      }
      const idForRequest = id;
      update(targetKey, (state) => ({
        draft: '',
        creating: false,
        messages: [
          ...state.messages,
          {
            id: localUserID,
            conversation_id: idForRequest,
            role: 'user',
            content: text,
            status: 'completed',
          },
          {
            id: assistantID,
            conversation_id: idForRequest,
            role: 'assistant',
            content: '',
            status: 'pending',
          },
        ],
      }));
      submitted = true;
      // 使用本次真实输入立即更新标题，停止生成后也能从列表找到该对话。
      setConversations((items) => {
        const item = items.find((conversation) => conversation.id === id);
        if (!item) return items;
        return [
          {
            ...item,
            title: item.title || Array.from(text).slice(0, 30).join(''),
            updated_at: new Date().toISOString(),
          },
          ...items.filter((conversation) => conversation.id !== id),
        ];
      });
      const idempotencyKey = crypto.randomUUID();
      const result = await streamRun(
        id,
        text,
        controller.signal,
        {
          onRunID: (runId, userMessageId) => {
            turn.runId = runId;
            update(targetKey, (state) => ({
              messages: state.messages.map((message) => {
                if (message.id === assistantID)
                  return { ...message, run_id: runId };
                if (message.id === localUserID && userMessageId)
                  return { ...message, id: userMessageId };
                return message;
              }),
            }));
            if (turn.stopRequested) void cancelTurn(targetKey, turn);
          },
          onMemoryUpdated: () => emitMemoryCue(turnAgentID, 'memory_updated'),
          onParts: (parts, content) => {
            update(targetKey, (state) => ({
              messages: state.messages.map((message) =>
                message.id === assistantID
                  ? { ...message, content, parts }
                  : message,
              ),
            }));
          },
        },
        idempotencyKey,
      );
      await finishTurn(targetKey, id, turn, result, turnAgentID, localUserID);
    } catch (cause) {
      if (controller.signal.aborted) return;
      const message = errorMessage(cause);
      update(targetKey, (state) => ({
        sendError: message,
        draft: submitted ? state.draft : text,
        messages: state.messages.map((item) =>
          item.id === assistantID && item.status === 'pending'
            ? {
                ...item,
                status: 'failed',
                error: message,
              }
            : item,
        ),
      }));
    } finally {
      if (turns.current.get(targetKey) === turn) {
        turns.current.delete(targetKey);
        update(targetKey, { sending: false, creating: false });
      }
    }
  };

  const current = sessions.get(key) || emptySession;
  const [pendingApproval] = pendingApprovals(
    current.messages.flatMap((message) =>
      message.status === 'pending' ? (message.parts ?? []) : [],
    ),
  );
  const loadOlder = async () => {
    const state = cache.current.get(key) || emptySession;
    if (
      !conversationID ||
      !state.hasMore ||
      state.loading ||
      state.loadingOlder ||
      !state.before
    )
      return;
    const controller = new AbortController();
    controllers.current.set(`older:${key}`, controller);
    update(key, { loadingOlder: true, olderError: '' });
    try {
      const result = await listMessages(
        conversationID,
        controller.signal,
        state.before,
      );
      if (controller.signal.aborted) return;
      update(key, (latest) => {
        const known = new Set(latest.messages.map((message) => message.id));
        return {
          messages: [
            ...result.messages.filter((message) => !known.has(message.id)),
            ...latest.messages,
          ],
          hasMore: Boolean(result.has_more),
          before: result.before,
        };
      });
    } catch (cause) {
      if (!controller.signal.aborted)
        update(key, { olderError: errorMessage(cause) });
    } finally {
      if (controllers.current.get(`older:${key}`) === controller) {
        controllers.current.delete(`older:${key}`);
        update(key, { loadingOlder: false });
      }
    }
  };

  return {
    ...current,
    pendingApproval,
    key,
    agentID,
    conversationID,
    conversations,
    listLoading,
    listError,
    activeConversation: conversations.find(
      (item) => item.id === conversationID,
    ),
    selectAgent: (agent: Agent) => {
      const recent = conversations.find((item) => item.agent_id === agent.id);
      history.push(chatURL(agent.id, recent?.id));
    },
    updateAgent: (agent: Agent) => {
      for (const [id, state] of cache.current) {
        if (state.agent?.id === agent.id) update(id, { agent });
      }
    },
    send,
    stop: async () => {
      const turn = turns.current.get(key);
      if (!turn || turn.stopRequested || turn.finished) return;
      // 首帧还未携带 run ID 时，保持订阅并在收到 ID 后取消。
      turn.stopRequested = true;
      await cancelTurn(key, turn);
    },
    setDraft: (draft: string) => update(key, { draft }),
    retry: () => void load(),
    retryList: () => setRevision((value) => value + 1),
    loadOlder,
    memoryCue,
  };
}
