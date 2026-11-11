import * as Tabs from '@radix-ui/react-tabs';
import { useEffect, useEffectEvent, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  type Agent,
  type AgentVersion,
  errorMessage,
  getAgentConfig,
  saveAgent,
} from '@/services/api';
import { AgentActivity } from './AgentActivity';
import { AgentTools } from './AgentTools';
import type { PendingApproval } from './approval-copy';
import {
  type MemoryCue,
  MemoryDetail,
  MemoryRows,
  useAgentMemory,
} from './MemoryPanel';

type Props = {
  agent: Agent;
  userID?: string;
  memoryCue: MemoryCue;
  approvals: PendingApproval[];
  onShowApproval: () => void;
  onEdit: () => void;
  onClose?: () => void;
  onSaved: (agent: Agent) => void;
};

/** 对话侧栏显示 Agent 动态、记忆与设置。配置保存沿用版本校验。 */
export function AgentDetails({
  agent,
  userID,
  memoryCue,
  approvals,
  onShowApproval,
  onEdit,
  onClose,
  onSaved,
}: Props) {
  const [tab, setTab] = useState('activity');
  const [version, setVersion] = useState<AgentVersion>();
  const [error, setError] = useState('');
  const [description, setDescription] = useState(agent.description);
  const [soul, setSoul] = useState('');
  const [model, setModel] = useState('');
  const [editing, setEditing] = useState(false);
  const [preview, setPreview] = useState(false);
  const [saving, setSaving] = useState(false);
  const [revision, setRevision] = useState(0);
  const editable = agent.owner_type === 'user' && agent.owner_id === userID;
  const current = version?.id === agent.current_version_id;
  const applyVersion = useEffectEvent((result: AgentVersion) => {
    setVersion(result);
    setSoul(result.configuration.soul_md);
    setModel(result.configuration.model);
    if (result.id !== agent.current_version_id) {
      onSaved({ ...agent, current_version_id: result.id });
    }
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: Agent 版本变化及冲突后重新读取配置。
  useEffect(() => {
    const controller = new AbortController();
    getAgentConfig(agent.id, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        applyVersion(result);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agent.id, agent.current_version_id, revision]);
  useEffect(() => setDescription(agent.description), [agent.description]);

  const memory = useAgentMemory({
    agentId: agent.id,
    version: current ? version : undefined,
    editable: editable && current,
    cue: memoryCue,
    onPolicySaved: (result) =>
      onSaved({ ...agent, current_version_id: result.current_version_id }),
    onConflict: () => setRevision((value) => value + 1),
  });
  const save = async (values: Parameters<typeof saveAgent>[2]) => {
    if (!version || !current || !editable || saving) return;
    setSaving(true);
    setError('');
    try {
      const result = await saveAgent(agent, version, values);
      setEditing(false);
      onSaved(result.agent);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  const alert = error && (
    <p role="alert" className="mx-4 my-2 text-xs text-rose-700">
      {error}{' '}
      <button
        type="button"
        className="underline"
        onClick={() => {
          setError('');
          setRevision((value) => value + 1);
        }}
      >
        重新读取
      </button>
    </p>
  );

  if (memory.detailOpen) {
    return (
      <MemoryDetail
        memory={memory}
        agentId={agent.id}
        onBack={() => memory.setDetailOpen(false)}
      />
    );
  }

  if (editing && version) {
    return (
      <section
        className="flex h-full min-h-0 flex-col bg-white"
        aria-label="Agent 配置"
      >
        <header className="flex shrink-0 items-center gap-3 px-4 py-3">
          <button
            type="button"
            className="inline-flex items-center gap-1 text-sm text-zinc-600"
            onClick={() => {
              setSoul(version.configuration.soul_md);
              setEditing(false);
            }}
          >
            <Icon name="arrow-left" size={16} />
            返回
          </button>
          <h2 className="text-sm font-medium">SOUL</h2>
          <div className="ml-auto flex items-center gap-2">
            <button
              type="button"
              className="text-xs text-zinc-600 hover:underline"
              onClick={() => setPreview(!preview)}
            >
              {preview ? '编辑' : '预览'}
            </button>
            <Button
              size="sm"
              disabled={
                saving || !current || soul === version.configuration.soul_md
              }
              onClick={() => void save({ soul_md: soul })}
            >
              {saving ? '保存中…' : '保存'}
            </Button>
          </div>
        </header>
        {alert}
        <div className="min-h-0 flex-1 bg-zinc-100">
          {preview ? (
            <div className="soul-preview h-full overflow-auto px-5 py-4 text-sm text-zinc-800">
              <Markdown>{soul}</Markdown>
            </div>
          ) : (
            <textarea
              className="soul-editor h-full w-full resize-none bg-transparent px-5 py-4 font-mono text-[13px] leading-6"
              aria-label="SOUL.md"
              maxLength={20000}
              value={soul}
              disabled={saving}
              onChange={(event) => setSoul(event.target.value)}
            />
          )}
        </div>
      </section>
    );
  }

  return (
    <section
      className="relative flex h-full min-h-0 flex-col bg-white text-sm"
      aria-label="Agent 面板"
    >
      {onClose && (
        <button
          type="button"
          className="absolute top-3 right-3 flex size-8 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100"
          onClick={onClose}
          aria-label="收起 Agent 配置"
        >
          <Icon name="close" size={18} />
        </button>
      )}
      <header className="flex shrink-0 flex-col items-center px-12 pt-6 pb-4 text-center">
        <div className="relative">
          <AgentAvatar size={56} />
          {editable && (
            <button
              type="button"
              className="absolute -right-1 -bottom-1 flex size-6 items-center justify-center rounded-full border border-zinc-200 bg-white shadow-sm"
              aria-label="编辑基本信息"
              onClick={onEdit}
            >
              <Icon name="edit" size={12} />
            </button>
          )}
        </div>
        <h2 className="mt-2 break-all text-base font-semibold">{agent.name}</h2>
        <p
          role="status"
          className={
            approvals.length
              ? 'mt-0.5 text-xs text-amber-800'
              : 'mt-0.5 text-xs text-zinc-500'
          }
        >
          {approvals.length
            ? `有 ${approvals.length} 项待你确认`
            : agent.status === 'active'
              ? '在线'
              : '已停用'}
        </p>
      </header>
      <Tabs.Root
        value={tab}
        onValueChange={setTab}
        className="flex min-h-0 flex-1 flex-col"
      >
        <Tabs.List
          aria-label="Agent 面板"
          className="flex shrink-0 gap-6 border-b border-zinc-200 px-4"
        >
          {(['动态', '记忆', '设置'] as const).map((label, index) => (
            <Tabs.Trigger
              key={label}
              value={['activity', 'memory', 'settings'][index]}
              className="border-b-2 border-transparent py-3 text-zinc-500 data-[state=active]:border-zinc-900 data-[state=active]:text-zinc-900"
            >
              {label}
            </Tabs.Trigger>
          ))}
        </Tabs.List>
        <Tabs.Content value="activity" className="min-h-0 flex-1 overflow-auto">
          <AgentActivity
            agentId={agent.id}
            approvals={approvals}
            onShowApproval={onShowApproval}
            owned={editable}
          />
        </Tabs.Content>
        <Tabs.Content
          value="memory"
          className="min-h-0 flex-1 overflow-auto px-4 pb-4"
        >
          {alert}
          <MemoryRows memory={memory} />
        </Tabs.Content>
        <Tabs.Content
          value="settings"
          className="min-h-0 flex-1 overflow-auto px-4 pt-4 pb-5"
        >
          {alert}
          <textarea
            aria-label="简介"
            placeholder="填写简介"
            value={description}
            readOnly={!editable}
            disabled={saving}
            onChange={(event) => setDescription(event.target.value)}
            className="block min-h-16 w-full resize-y rounded-xl bg-zinc-50 px-3 py-2.5 leading-5"
          />
          {editable && description !== agent.description && (
            <Button
              className="mt-2"
              size="sm"
              disabled={!current || saving}
              onClick={() => void save({ description })}
            >
              保存简介
            </Button>
          )}
          <AgentTools
            agent={agent}
            onSaved={onSaved}
            editable={editable}
            advanced={
              <>
                {!version && !error && <Skeleton className="h-20" />}
                {version && (
                  <>
                    <div className="flex min-h-10 items-center justify-between gap-2">
                      <span className="text-zinc-500">模型</span>
                      {editable ? (
                        <Select
                          value={model}
                          disabled={!current || saving}
                          onValueChange={setModel}
                        >
                          <SelectTrigger
                            className="h-auto w-auto border-0 bg-transparent p-0 text-sm"
                            aria-label="模型"
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {version.available_models.map((name) => (
                              <SelectItem key={name} value={name}>
                                {name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <span>{model}</span>
                      )}
                    </div>
                    {editable && model !== version.configuration.model && (
                      <Button
                        size="sm"
                        disabled={!current || saving}
                        onClick={() => void save({ model })}
                      >
                        保存模型
                      </Button>
                    )}
                    <div className="flex min-h-10 items-center justify-between gap-2">
                      <span className="text-zinc-500">SOUL</span>
                      <span className="flex items-center gap-3">
                        v{version.version_number}
                        {editable && (
                          <button
                            type="button"
                            className="text-zinc-500 hover:underline"
                            aria-label="编辑 SOUL.md"
                            disabled={!current || saving}
                            onClick={() => {
                              setPreview(false);
                              setEditing(true);
                            }}
                          >
                            编辑
                          </button>
                        )}
                      </span>
                    </div>
                  </>
                )}
              </>
            }
          />
        </Tabs.Content>
      </Tabs.Root>
    </section>
  );
}
