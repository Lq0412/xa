import { useEffect, useRef, useState } from 'react';
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
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  type Agent,
  type AgentVersion,
  errorMessage,
  getAgentConfig,
  saveAgent,
} from '@/services/api';

type Props = {
  agent: Agent;
  userID?: string;
  onEdit: () => void;
  onClose?: () => void;
  onSaved: (agent: Agent) => void;
};

function cardDate(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const pad = (item: number) => String(item).padStart(2, '0');
  return `${pad(date.getMonth() + 1)}.${pad(date.getDate())}.${String(date.getFullYear()).slice(2)}`;
}

function statusText(agent: Agent) {
  const role = agent.is_primary
    ? '主 Agent'
    : agent.visibility === 'public'
      ? '公共 Agent'
      : '私有 Agent';
  return agent.status === 'active' ? role : `${role} · 已停用`;
}

/** 默认阅读配置。SOUL 编辑占满整个配置栏；保存使用不可变版本与乐观锁。 */
export function AgentDetails({
  agent,
  userID,
  onEdit,
  onClose,
  onSaved,
}: Props) {
  const [version, setVersion] = useState<AgentVersion>();
  const [error, setError] = useState('');
  const [soul, setSoul] = useState('');
  const [model, setModel] = useState('');
  const [editing, setEditing] = useState(false);
  const [preview, setPreview] = useState(false);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);
  const savedSoul = useRef(false);
  const editable = agent.owner_type === 'user' && agent.owner_id === userID;
  // biome-ignore lint/correctness/useExhaustiveDependencies: 版本指针和 revision 驱动外部编辑及重试后的读取。
  useEffect(() => {
    const controller = new AbortController();
    setError('');
    setVersion(undefined);
    setEditing(false);
    getAgentConfig(agent.id, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setVersion(result);
          setSoul(result.configuration.soul_md);
          setModel(result.configuration.model);
          if (savedSoul.current) {
            savedSoul.current = false;
            setNotice(`已保存为 v${result.version_number}`);
          }
        }
      })
      .catch((cause) => {
        if (!controller.signal.aborted) {
          savedSoul.current = false;
          setError(errorMessage(cause));
        }
      });
    return () => controller.abort();
  }, [agent.id, agent.current_version_id, revision]);
  const save = async (field: 'soul_md' | 'model') => {
    if (!version || !editable || saving) return;
    setSaving(true);
    setError('');
    setNotice('');
    try {
      const result = await saveAgent(
        agent,
        version,
        field === 'soul_md' ? { soul_md: soul } : { model },
      );
      if (field === 'soul_md') savedSoul.current = true;
      else setNotice('已保存');
      setEditing(false);
      onSaved(result.agent);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  const openSoul = () => {
    setEditing(true);
    setPreview(false);
    setNotice('');
  };
  const closeSoul = () => {
    if (version) setSoul(version.configuration.soul_md);
    setPreview(false);
    setEditing(false);
  };
  const date = cardDate(agent.created_at);
  const soulCard = (
    <>
      <span className="text-sm font-medium">SOUL</span>
      <span className="version-label mt-1 text-xs text-zinc-800">{`v${version?.version_number ?? ''}`}</span>
      <span className="mt-auto pt-6 text-[11px] text-zinc-700">{date}</span>
    </>
  );
  const soulStyle = {
    background: 'linear-gradient(160deg, #ffe08a 0%, #fff6d0 100%)',
  };
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
            onClick={closeSoul}
          >
            <Icon name="arrow-left" size={16} />
            返回
          </button>
          <h2 className="text-sm font-medium">SOUL</h2>
          <div className="ml-auto flex items-center gap-2">
            <div className="flex rounded-full bg-zinc-100 p-0.5 text-xs">
              <button
                type="button"
                className={`rounded-full px-2.5 py-1 ${preview ? 'text-zinc-500' : 'bg-white text-zinc-900 shadow-sm'}`}
                aria-pressed={!preview}
                onClick={() => setPreview(false)}
              >
                编辑
              </button>
              <button
                type="button"
                className={`rounded-full px-2.5 py-1 ${preview ? 'bg-white text-zinc-900 shadow-sm' : 'text-zinc-500'}`}
                aria-pressed={preview}
                onClick={() => setPreview(true)}
              >
                预览
              </button>
            </div>
            <Button
              size="sm"
              disabled={saving || soul === version.configuration.soul_md}
              onClick={() => void save('soul_md')}
            >
              {saving ? '保存中…' : '保存'}
            </Button>
          </div>
        </header>
        {error && (
          <div
            className="mx-4 mb-2 flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3 py-2 text-sm text-rose-800"
            role="alert"
          >
            {error}
            <button
              type="button"
              className="shrink-0 text-xs underline"
              disabled={saving}
              onClick={() => setRevision((value) => value + 1)}
            >
              重新读取
            </button>
          </div>
        )}
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
      className="relative flex h-full min-h-0 flex-col"
      aria-label="Agent 配置"
    >
      {notice && (
        <p
          role="status"
          className="absolute top-14 left-1/2 z-10 -translate-x-1/2 rounded-full bg-zinc-900 px-3 py-1.5 text-xs whitespace-nowrap text-white shadow-sm"
        >
          {notice}
        </p>
      )}
      {onClose && (
        <button
          type="button"
          className="absolute top-3 right-3 z-10 flex size-8 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100"
          onClick={onClose}
          aria-label="收起 Agent 配置"
        >
          <Icon name="close" size={18} />
        </button>
      )}
      <div className="min-h-0 flex-1 overflow-auto px-5 pt-8 pb-6">
        <div className="flex flex-col items-center text-center">
          <div className="relative">
            <AgentAvatar size={96} />
            {editable && (
              <button
                type="button"
                className="absolute -right-1 -bottom-1 flex size-8 items-center justify-center rounded-full border border-zinc-200 bg-white shadow-sm"
                aria-label="编辑基本信息"
                onClick={onEdit}
              >
                <Icon name="edit" size={14} />
              </button>
            )}
          </div>
          <h2 className="mt-4 text-xl font-medium">{agent.name}</h2>
          {agent.description.trim() && (
            <p className="mt-1 line-clamp-2 max-w-full px-2 text-xs leading-5 text-zinc-500">
              {agent.description}
            </p>
          )}
        </div>
        {error && (
          <div
            className="mt-4 flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3 py-2 text-sm text-rose-800"
            role="alert"
          >
            {error}
            <button
              type="button"
              className="shrink-0 text-xs underline"
              disabled={saving}
              onClick={() => setRevision((value) => value + 1)}
            >
              重新读取
            </button>
          </div>
        )}
        {!version ? (
          !error && (
            <div className="mt-6 space-y-3" role="status">
              <span className="sr-only">正在读取配置</span>
              <Skeleton className="h-24 w-full" />
              <div className="grid grid-cols-2 gap-3">
                <Skeleton className="h-36" />
                <Skeleton className="h-36" />
              </div>
            </div>
          )
        ) : (
          <Tabs defaultValue="modules" className="mt-6">
            <TabsList>
              <TabsTrigger value="modules">模块</TabsTrigger>
              <TabsTrigger value="about">详情</TabsTrigger>
            </TabsList>
            <TabsContent value="modules" className="mt-4">
              <div className="rounded-2xl border border-zinc-200 bg-white p-4 text-left">
                <p className="line-clamp-4 text-sm leading-6 whitespace-pre-wrap text-zinc-600">
                  {soul || '未设置'}
                </p>
                {editable && (
                  <button
                    type="button"
                    className="mt-3 text-sm text-zinc-800 underline underline-offset-4"
                    aria-label="编辑 SOUL.md"
                    onClick={openSoul}
                  >
                    编辑
                  </button>
                )}
              </div>
              <Separator className="my-4" />
              <div className="grid grid-cols-2 items-stretch gap-3">
                {editable ? (
                  <button
                    type="button"
                    className="flex min-h-36 flex-col rounded-[20px] p-3 text-left"
                    style={soulStyle}
                    onClick={openSoul}
                    disabled={saving}
                  >
                    {soulCard}
                  </button>
                ) : (
                  <div
                    className="flex min-h-36 flex-col rounded-[20px] p-3 text-left"
                    style={soulStyle}
                  >
                    {soulCard}
                  </div>
                )}
                <div
                  className="flex min-h-36 flex-col rounded-[20px] p-3 text-left"
                  style={{
                    background:
                      'linear-gradient(160deg, #d7e6ff 0%, #f4f8ff 100%)',
                  }}
                >
                  <span className="text-sm font-medium">模型</span>
                  <span className="mt-1 truncate text-xs text-zinc-700">
                    {model}
                  </span>
                  {editable && (
                    <Select
                      value={model}
                      disabled={saving}
                      onValueChange={(value) => {
                        setModel(value);
                        setNotice('');
                      }}
                    >
                      <SelectTrigger className="mt-2" aria-label="模型">
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
                  )}
                  <span className="mt-auto pt-4 text-[11px] text-zinc-600">
                    {date}
                  </span>
                  {editable && model !== version.configuration.model && (
                    <Button
                      className="mt-2"
                      size="sm"
                      disabled={saving}
                      onClick={() => void save('model')}
                    >
                      保存
                    </Button>
                  )}
                </div>
              </div>
            </TabsContent>
            <TabsContent
              value="about"
              className="mt-4 space-y-4 text-left text-sm"
            >
              <div>
                <p className="text-xs text-zinc-400">简介</p>
                <p className="mt-1 leading-6 text-zinc-700">
                  {agent.description || '未填写'}
                </p>
              </div>
              <div>
                <p className="text-xs text-zinc-400">状态</p>
                <p className="mt-1 text-zinc-700">{statusText(agent)}</p>
              </div>
              {agent.created_at && (
                <p className="text-xs text-zinc-400">
                  创建于{' '}
                  <time dateTime={agent.created_at}>
                    {new Date(agent.created_at).toLocaleDateString('zh-CN')}
                  </time>
                </p>
              )}
              {(agent.tags ?? []).length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {(agent.tags ?? []).map((tag) => (
                    <span
                      key={tag}
                      className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600"
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              )}
            </TabsContent>
          </Tabs>
        )}
      </div>
    </section>
  );
}
