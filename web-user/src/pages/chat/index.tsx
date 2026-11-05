import { history, useLocation, useModel } from '@umijs/max';
import { useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
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

function oneLine(value: string) {
  return value.replace(/\s+/g, ' ').trim();
}

/** 四栏分别负责导航、Agent 选择、消息和配置；窄屏使用 dialog 隔离焦点。 */
export default function ChatPage() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const library = useAgentLibrary(params.get('agentId') || params.get('agent'));
  const session = useChatSession(location.search, library.agents[0]);
  const { initialState } = useModel('@@initialState');
  const sidebarInline = useMediaQuery('(min-width: 900px)');
  const detailsInline = useMediaQuery('(min-width: 1200px)');
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const details = useDetailsPreference();
  const [editor, setEditor] = useState<{ agent?: Agent }>();
  const saved = (agent: Agent) => {
    library.refresh();
    session.updateAgent(agent);
  };
  const livePreview = oneLine(
    [...session.messages].reverse().find((item) => item.content.trim())
      ?.content || '',
  );
  const sidebar = (
    <AgentSidebar
      agents={library.agents}
      activeAgentID={session.agent?.id || session.agentID || undefined}
      userID={initialState?.currentUser?.id}
      conversations={session.conversations}
      liveAgentID={session.agent?.id}
      livePreview={livePreview}
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
  const openCreate = () => {
    setMobileSidebarOpen(false);
    setEditor({});
  };
  return (
    <div className="flex h-full min-h-0 flex-1">
      {sidebarInline ? (
        <aside
          className="agent-sidebar flex h-full w-[260px] shrink-0 flex-col border-r border-zinc-200 bg-[#f4f4f5] px-3 py-3"
          aria-label="对话 Agent"
        >
          {sidebar}
        </aside>
      ) : (
        mobileSidebarOpen && (
          <Dialog
            title="我的 Agent"
            side="left"
            onClose={() => setMobileSidebarOpen(false)}
          >
            <div className="flex min-h-0 flex-1 flex-col px-3 pb-4">
              {sidebar}
            </div>
          </Dialog>
        )
      )}
      <section className="conversation-main relative flex min-w-0 flex-1 flex-col bg-white">
        <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex items-start justify-between px-3 pt-3">
          <div className="pointer-events-auto flex w-10 justify-start">
            {!sidebarInline && (
              <button
                type="button"
                className="flex size-9 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100"
                aria-label="打开 Agent 列表"
                onClick={() => {
                  details.setOpen(false);
                  setMobileSidebarOpen(true);
                }}
              >
                <Icon name="menu" />
              </button>
            )}
          </div>
          <div className="pointer-events-auto flex max-w-[min(280px,46%)] items-center gap-2 rounded-full border border-zinc-200 bg-white/95 px-2 py-1 shadow-sm">
            {session.agent && <AgentAvatar size={24} />}
            <strong className="conversation-title min-w-0 truncate text-sm font-medium">
              {session.agent?.name || '选择 Agent'}
            </strong>
          </div>
          <div className="pointer-events-auto flex w-10 justify-end">
            <button
              type="button"
              className="flex size-9 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100 disabled:opacity-40"
              aria-label={details.open ? '收起 Agent 配置' : '展开 Agent 配置'}
              title="Agent 配置"
              aria-expanded={details.open}
              disabled={!session.agent}
              onClick={() => {
                setMobileSidebarOpen(false);
                details.setOpen(!details.open);
              }}
            >
              <Icon name="panel" size={18} />
            </button>
          </div>
        </div>
        <div className="flex min-h-0 flex-1 flex-col pt-14">
          {(session.listError || session.sendError) && (
            <div
              className="mx-auto mb-2 flex w-full max-w-[760px] shrink-0 items-center justify-between gap-3 rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-800"
              role="alert"
            >
              {session.listError || session.sendError}
              <button
                type="button"
                className="shrink-0 underline"
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
            canSuggest={
              Boolean(session.agent) &&
              session.loaded &&
              !session.loading &&
              !session.sending &&
              session.agent?.status === 'active' &&
              !session.listError
            }
            onSuggest={(text) => {
              // setDraft 同步写入会话缓存，send 能立刻读到这条建议。
              session.setDraft(text);
              void session.send();
            }}
            onRetry={session.retry}
            onLoadOlder={session.loadOlder}
            hasMore={session.hasMore}
            loadingOlder={session.loadingOlder}
            olderError={session.olderError}
          />
          {session.agent && session.agent.status !== 'active' && (
            <p className="shrink-0 text-center text-xs text-amber-700">
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
            onCreate={openCreate}
          />
        </div>
      </section>
      {panel &&
        details.open &&
        (detailsInline ? (
          <aside
            className="agent-details-panel h-full w-[340px] shrink-0 border-l border-zinc-200 bg-white"
            aria-label="Agent 配置栏"
          >
            {panel}
          </aside>
        ) : (
          <Dialog
            title="Agent 配置"
            side="right"
            onClose={() => details.setOpen(false)}
          >
            <div className="flex min-h-0 flex-1 flex-col">{panel}</div>
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
