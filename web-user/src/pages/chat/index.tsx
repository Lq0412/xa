import {
  InfoCircleOutlined,
  MenuOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { history, useLocation, useModel } from '@umijs/max';
import { Alert, Avatar, Button, Drawer, Grid } from 'antd';
import { useState } from 'react';
import { AgentEditor } from '@/components/AgentEditor';
import { useDetailsPreference } from '@/layouts/DetailsPreference';
import type { Agent } from '@/services/api';
import { chatURL } from '@/services/chat';
import { AgentDetails } from './AgentDetails';
import { AgentSidebar } from './AgentSidebar';
import { MessageComposer } from './MessageComposer';
import { MessageList } from './MessageList';
import { useAgentLibrary } from './useAgentLibrary';
import { useChatSession } from './useChatSession';
import './chat.css';

/** 对话页面只组合自定义界面，数据请求和消息更新由 useChatSession 管理。 */
export default function ChatPage() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const library = useAgentLibrary(params.get('agentId') || params.get('agent'));
  const session = useChatSession(location.search, library.agents[0]);
  const { initialState } = useModel('@@initialState');
  const screens = Grid.useBreakpoint();
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const details = useDetailsPreference();
  const [editor, setEditor] = useState<{ agent?: Agent }>();

  const sidebar = (
    <AgentSidebar
      agents={library.agents}
      conversations={session.conversations}
      activeAgentID={session.agent?.id || session.agentID}
      activeConversationID={session.conversationID}
      loading={library.loading}
      historyLoading={session.listLoading}
      error={library.error}
      onRetry={library.refresh}
      onCreate={() => {
        setMobileSidebarOpen(false);
        setEditor({});
      }}
      onSelectAgent={(agent) => {
        session.selectAgent(agent);
        setMobileSidebarOpen(false);
      }}
      onNewConversation={(agent) => {
        session.newConversation(agent);
        setMobileSidebarOpen(false);
      }}
      onSelectConversation={(id) => {
        session.selectConversation(id);
        setMobileSidebarOpen(false);
      }}
    />
  );
  return (
    <div className="chat-layout">
      {screens.md ? (
        <aside className="agent-sidebar" aria-label="对话 Agent">
          {sidebar}
        </aside>
      ) : (
        <Drawer
          title="Agent"
          placement="left"
          size={300}
          open={mobileSidebarOpen}
          onClose={() => setMobileSidebarOpen(false)}
          closable={{ 'aria-label': '关闭 Agent 列表' }}
          styles={{ body: { padding: 14 } }}
        >
          {sidebar}
        </Drawer>
      )}
      <section className="conversation-main">
        <header className="conversation-header">
          {!screens.md && (
            <button
              type="button"
              className="chat-icon-button"
              aria-label="打开 Agent 列表"
              onClick={() => {
                details.setOpen(false);
                setMobileSidebarOpen(true);
              }}
            >
              <MenuOutlined />
            </button>
          )}
          <div className="conversation-identity">
            <Avatar
              size={36}
              src={session.agent?.avatar || undefined}
              icon={<RobotOutlined />}
            />
            <div>
              <strong>{session.agent?.name || '选择 Agent'}</strong>
            </div>
          </div>
          <Button
            type="text"
            icon={<InfoCircleOutlined />}
            aria-label={details.open ? '收起 Agent 详情' : '展开 Agent 详情'}
            aria-expanded={details.open}
            disabled={!session.agent}
            onClick={() => {
              setMobileSidebarOpen(false);
              details.setOpen(!details.open);
            }}
          />
        </header>
        {(session.listError || session.sendError) && (
          <Alert
            className="chat-error"
            type="error"
            showIcon
            title={session.listError || session.sendError}
            action={
              <Button
                size="small"
                onClick={session.listError ? session.retryList : session.retry}
                disabled={session.sending}
              >
                重新加载记录
              </Button>
            }
          />
        )}
        <MessageList
          key={session.key}
          agent={session.agent}
          messages={session.messages}
          loading={session.loading || session.listLoading}
          error={session.loadError}
          onRetry={session.retry}
        />
        {session.agent && session.agent.status !== 'active' && (
          <p className="chat-unavailable">
            此 Agent 已停用，可以查看历史消息。
          </p>
        )}
        <MessageComposer
          value={session.draft}
          disabled={
            !session.loaded ||
            session.loading ||
            session.agent?.status !== 'active' ||
            Boolean(session.listError)
          }
          sending={session.sending}
          creating={session.creating}
          onChange={session.setDraft}
          onSend={() => void session.send()}
          onStop={session.stop}
        />
      </section>
      {session.agent &&
        (screens.xl ? (
          details.open && (
            <aside className="agent-details-panel" aria-label="Agent 详情栏">
              <AgentDetails
                key={session.agent.id}
                agent={session.agent}
                userID={initialState?.currentUser?.id}
                onEdit={() => setEditor({ agent: session.agent })}
                onClose={() => details.setOpen(false)}
              />
            </aside>
          )
        ) : (
          <Drawer
            open={details.open}
            onClose={() => details.setOpen(false)}
            size={320}
            closable={false}
            styles={{ body: { padding: 0 } }}
          >
            <AgentDetails
              key={session.agent.id}
              agent={session.agent}
              userID={initialState?.currentUser?.id}
              onEdit={() => {
                details.setOpen(false);
                setEditor({ agent: session.agent });
              }}
              onClose={() => details.setOpen(false)}
            />
          </Drawer>
        ))}
      {editor && (
        <AgentEditor
          agent={editor.agent}
          onCancel={() => setEditor(undefined)}
          onSaved={(agent) => {
            setEditor(undefined);
            library.refresh();
            session.updateAgent(agent);
            if (!editor.agent) history.push(chatURL(agent.id));
          }}
        />
      )}
    </div>
  );
}
