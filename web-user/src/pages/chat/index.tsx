import { history, useLocation, useModel } from '@umijs/max';
import { useState } from 'react';
import { AgentEditor } from '@/components/AgentEditor';
import { Dialog } from '@/components/Dialog';
import { Icon } from '@/components/Icon';
import { useMediaQuery } from '@/components/useMediaQuery';
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

/** 四栏分别负责导航、Agent 选择、消息和配置；窄屏使用原生 dialog 隔离焦点。 */
export default function ChatPage() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const library = useAgentLibrary(params.get('agentId') || params.get('agent'));
  const session = useChatSession(location.search, library.agents[0]);
  const { initialState } = useModel('@@initialState');
  const sidebarInline = useMediaQuery('(min-width: 768px)');
  const detailsInline = useMediaQuery('(min-width: 1200px)');
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const details = useDetailsPreference();
  const [editor, setEditor] = useState<{ agent?: Agent }>();
  const saved = (agent: Agent) => {
    library.refresh();
    session.updateAgent(agent);
  };
  const sidebar = (
    <AgentSidebar
      agents={library.agents}
      activeAgentID={session.agent?.id || session.agentID || undefined}
      loading={library.loading}
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
    />
  );
  const panel = session.agent && (
    <AgentDetails
      key={session.agent.id}
      agent={session.agent}
      userID={initialState?.currentUser?.id}
      onEdit={() => {
        if (!detailsInline) details.setOpen(false);
        setEditor({ agent: session.agent });
      }}
      onClose={detailsInline ? () => details.setOpen(false) : undefined}
      onSaved={saved}
    />
  );
  return (
    <div className="chat-layout">
      {sidebarInline ? (
        <aside className="agent-sidebar" aria-label="对话 Agent">
          {sidebar}
        </aside>
      ) : (
        mobileSidebarOpen && (
          <Dialog
            title="我的 Agent"
            side="left"
            onClose={() => setMobileSidebarOpen(false)}
          >
            <div className="sheet-sidebar">{sidebar}</div>
          </Dialog>
        )
      )}
      <section className="conversation-main">
        <header className="conversation-header">
          {!sidebarInline && (
            <button
              type="button"
              className="icon-button"
              aria-label="打开 Agent 列表"
              onClick={() => {
                details.setOpen(false);
                setMobileSidebarOpen(true);
              }}
            >
              <Icon name="menu" />
            </button>
          )}
          <strong className="conversation-title">
            {session.agent?.name || '选择 Agent'}
          </strong>
          <button
            type="button"
            className="icon-button"
            aria-label={details.open ? '收起 Agent 配置' : '展开 Agent 配置'}
            title="Agent 配置"
            aria-expanded={details.open}
            disabled={!session.agent}
            onClick={() => {
              setMobileSidebarOpen(false);
              details.setOpen(!details.open);
            }}
          >
            <Icon name="info" size={19} />
          </button>
        </header>
        {(session.listError || session.sendError) && (
          <div className="error-banner chat-error" role="alert">
            {session.listError || session.sendError}
            <button
              type="button"
              className="text-button"
              disabled={session.sending}
              onClick={session.listError ? session.retryList : session.retry}
            >
              重新加载
            </button>
          </div>
        )}
        <MessageList
          key={session.key}
          agent={session.agent}
          messages={session.messages}
          loading={session.loading || session.listLoading}
          error={session.loadError}
          onRetry={session.retry}
          onLoadOlder={session.loadOlder}
          hasMore={session.hasMore}
          loadingOlder={session.loadingOlder}
          olderError={session.olderError}
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
      {panel &&
        details.open &&
        (detailsInline ? (
          <aside className="agent-details-panel" aria-label="Agent 配置栏">
            {panel}
          </aside>
        ) : (
          <Dialog
            title="Agent 配置"
            side="right"
            onClose={() => details.setOpen(false)}
          >
            {panel}
          </Dialog>
        ))}
      {editor && (
        <AgentEditor
          agent={editor.agent}
          onCancel={() => setEditor(undefined)}
          onSaved={(agent) => {
            setEditor(undefined);
            saved(agent);
            if (!editor.agent) history.push(chatURL(agent.id));
          }}
        />
      )}
    </div>
  );
}
