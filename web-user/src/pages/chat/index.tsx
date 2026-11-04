import {
  ArrowUpOutlined,
  PlusOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { useLocation } from '@umijs/max';
import { Alert, Avatar, Button, Input, List, Spin } from 'antd';
import { useEffect, useState } from 'react';
import {
  type Agent,
  type ChatMessage,
  type Conversation,
  createConversation,
  errorMessage,
  getAgent,
  listConversations,
  listMessages,
  streamMessage,
} from '@/services/api';

/** 用户端对话页负责会话选择、历史恢复和 SSE 增量展示。 */
export default function ChatPage() {
  const agentID = new URLSearchParams(useLocation().search).get('agent');
  const [agent, setAgent] = useState<Agent>();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [conversation, setConversation] = useState<Conversation>();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    if (agentID)
      getAgent(agentID)
        .then(setAgent)
        .catch((e) => setError(errorMessage(e)));
    listConversations()
      .then((r) => setConversations(r.conversations))
      .catch((e) => setError(errorMessage(e)));
  }, [agentID]);
  useEffect(() => {
    if (conversation)
      listMessages(conversation.id)
        .then((r) => setMessages(r.messages))
        .catch((e) => setError(errorMessage(e)));
  }, [conversation]);
  const newChat = async () => {
    if (!agent) return;
    const c = await createConversation(agent.id);
    setConversations((v) => [c, ...v]);
    setConversation(c);
    setMessages([]);
  };
  const send = async () => {
    if (!conversation || !input.trim() || sending) return;
    const text = input.trim();
    setInput('');
    setMessages((v) => [
      ...v,
      {
        id: `local-${Date.now()}`,
        conversation_id: conversation.id,
        role: 'user',
        content: text,
        status: 'completed',
      },
      {
        id: `assistant-${Date.now()}`,
        conversation_id: conversation.id,
        role: 'assistant',
        content: '',
        status: 'pending',
      },
    ]);
    setSending(true);
    try {
      await streamMessage(
        conversation.id,
        text,
        (d) =>
          setMessages((v) => {
            const a = [...v];
            const i = a.length - 1;
            a[i] = { ...a[i], content: a[i].content + d };
            return a;
          }),
        () => {},
      );
      setMessages(await listMessages(conversation.id).then((r) => r.messages));
      setConversations(await listConversations().then((r) => r.conversations));
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSending(false);
    }
  };
  return (
    <div className="chat-layout">
      <aside className="conversation-sidebar">
        <Button type="primary" icon={<PlusOutlined />} onClick={newChat} block>
          新建会话
        </Button>
        <List
          dataSource={conversations}
          renderItem={(c) => (
            <List.Item
              onClick={() => setConversation(c)}
              style={{ cursor: 'pointer' }}
            >
              {c.title || '新对话'}
            </List.Item>
          )}
        />
      </aside>
      <section className="conversation-main">
        <header className="conversation-header">
          <Avatar icon={<RobotOutlined />} src={agent?.avatar || undefined} />
          <span>{agent?.name || '选择 Agent'}</span>
        </header>
        {error && (
          <Alert
            type="error"
            message={error}
            closable
            onClose={() => setError('')}
          />
        )}
        <div className="conversation-body">
          {!conversation ? (
            <div className="conversation-welcome">
              <Spin spinning={!agent} />
              <h1>{agent?.name || '选择 Agent 后开始对话'}</h1>
              <p>{agent?.description}</p>
            </div>
          ) : (
            messages.map((m) => (
              <div key={m.id} className={`chat-message ${m.role}`}>
                <b>{m.role === 'user' ? '你' : agent?.name || '助手'}</b>
                <div style={{ whiteSpace: 'pre-wrap' }}>{m.content}</div>
              </div>
            ))
          )}
        </div>
        <div className="composer-area">
          <Input.TextArea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onPressEnter={(e) => {
              if (!e.shiftKey) {
                e.preventDefault();
                send();
              }
            }}
            disabled={!conversation || sending}
            placeholder="输入消息，按 Enter 发送"
            autoSize={{ minRows: 1, maxRows: 5 }}
          />
          <Button
            type="primary"
            shape="circle"
            icon={<ArrowUpOutlined />}
            onClick={send}
            disabled={!conversation || sending || !input.trim()}
            loading={sending}
          />
        </div>
      </section>
    </div>
  );
}
