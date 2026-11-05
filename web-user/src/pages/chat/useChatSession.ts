import { history } from '@umijs/max';
import { useCallback, useEffect, useRef, useState } from 'react';
import { type Agent, getAgent } from '@/services/api';
import {
  type ChatMessage,
  type Conversation,
  chatURL,
  createConversation,
  listConversations,
  listMessages,
  streamMessage,
} from '@/services/chat';
import { errorMessage } from '@/services/http';
import type { MemoryCue } from './MemoryPanel';

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

  useEffect(() => {
    mounted.current = true;
    const requests = controllers.current;
    return () => {
      mounted.current = false;
      for (const request of requests.values()) request.abort();
      requests.clear();
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
    update(key, { loading: true, loadError: '' });
    try {
      const [agent, result] = await Promise.all([
        getAgent(targetAgent, controller.signal),
        conversationID
          ? listMessages(conversationID, controller.signal)
          : Promise.resolve({
              messages: [],
              has_more: false,
              before: undefined,
            }),
      ]);
      update(key, {
        agent,
        messages: result.messages ?? [],
        hasMore: Boolean(result.has_more),
        before: result.before,
        olderError: '',
        loaded: true,
        sendError: '',
      });
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
      current.loading
    )
      return;
    const controller = new AbortController();
    const turnAgentID = current.agent.id;
    let targetKey = key;
    controllers.current.set(`send:${key}`, controller);
    // 同步更新缓存，连续按 Enter 也只能发起一次请求。
    update(key, { sending: true, creating: !conversationID, sendError: '' });
    const assistantID = `local-assistant-${crypto.randomUUID()}`;
    const localUserID = `local-user-${crypto.randomUUID()}`;
    let submitted = false;
    let completed = false;
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
        controllers.current.delete(`send:${key}`);
        controllers.current.set(`send:${targetKey}`, controller);
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
      const savedID = await streamMessage(
        id,
        text,
        controller.signal,
        (delta) => {
          update(targetKey, (state) => ({
            messages: state.messages.map((message) =>
              message.id === assistantID
                ? { ...message, content: message.content + delta }
                : message,
            ),
          }));
        },
      );
      completed = true;
      // 抽取在回复落库后异步进行，详情栏稍后自己刷新一次。
      setMemoryCue((cue) => ({ agentId: turnAgentID, seq: cue.seq + 1 }));
      update(targetKey, (state) => ({
        messages: state.messages.map((message) =>
          message.id === assistantID
            ? { ...message, id: savedID, status: 'completed' }
            : message,
        ),
      }));
      const result = await listMessages(id, controller.signal);
      update(targetKey, (state) => {
        const latest = result.messages ?? [];
        const ids = new Set(latest.map((item) => item.id));
        const older = state.messages.filter(
          (item) =>
            !ids.has(item.id) &&
            item.id !== localUserID &&
            item.id !== assistantID,
        );
        return {
          messages: [...older, ...latest],
          hasMore: older.length ? state.hasMore : result.has_more,
          before: older.length ? state.before : result.before,
        };
      });
    } catch (cause) {
      const stopped = controller.signal.aborted;
      const message = stopped ? '已停止生成' : errorMessage(cause);
      update(targetKey, (state) => ({
        sendError: stopped ? '' : message,
        draft: submitted ? state.draft : text,
        messages: state.messages.map((item) =>
          item.id === assistantID && !completed
            ? {
                ...item,
                status: stopped ? 'stopped' : 'failed',
                error: message,
              }
            : item,
        ),
      }));
    } finally {
      update(targetKey, { sending: false, creating: false });
      controllers.current.delete(`send:${targetKey}`);
    }
  };

  const current = sessions.get(key) || emptySession;
  const loadOlder = async () => {
    const state = cache.current.get(key) || emptySession;
    if (
      !conversationID ||
      !state.hasMore ||
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
      update(key, (latest) => ({
        messages: [...(result.messages ?? []), ...latest.messages],
        hasMore: Boolean(result.has_more),
        before: result.before,
      }));
    } catch (cause) {
      if (!controller.signal.aborted)
        update(key, { olderError: errorMessage(cause) });
    } finally {
      update(key, { loadingOlder: false });
      controllers.current.delete(`older:${key}`);
    }
  };

  return {
    ...current,
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
    stop: () => controllers.current.get(`send:${key}`)?.abort(),
    setDraft: (draft: string) => update(key, { draft }),
    retry: () => void load(),
    retryList: () => setRevision((value) => value + 1),
    loadOlder,
    memoryCue,
  };
}
