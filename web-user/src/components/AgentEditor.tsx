import { type FormEvent, useEffect, useState } from 'react';
import { Dialog } from '@/components/Dialog';
import { Button } from '@/components/ui/button';
import {
  type Agent,
  type AgentVersion,
  ApiError,
  createAgent,
  errorMessage,
  getAgentConfig,
  saveAgent,
} from '@/services/api';

type Props = {
  agent?: Agent;
  onCancel: () => void;
  onSaved: (agent: Agent) => void;
};

const fieldClass =
  'w-full rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm outline-none focus:border-zinc-400';

/** 创建和基本资料编辑共用原生表单，配置由版本指针保护。 */
export function AgentEditor({ agent, onCancel, onSaved }: Props) {
  const [version, setVersion] = useState<AgentVersion>();
  const [name, setName] = useState(agent?.name || '');
  const [description, setDescription] = useState(agent?.description || '');
  const [soul, setSoul] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 是用户显式重新读取版本的触发器。
  useEffect(() => {
    if (!agent) return;
    const controller = new AbortController();
    getAgentConfig(agent.id, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setVersion(value);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agent?.id, revision]);
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (saving) return;
    setSaving(true);
    setError('');
    try {
      if (!name.trim()) throw new Error('名称不能为空');
      if (agent && !version) throw new ApiError(409, '请重新读取配置');
      const result =
        agent && version
          ? await saveAgent(agent, version, {
              name: name.trim(),
              description: description.trim(),
            })
          : await createAgent(name.trim(), description.trim(), soul);
      onSaved(result.agent);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Dialog
      title={agent ? '编辑基本信息' : '新建 Agent'}
      onClose={onCancel}
      busy={saving}
    >
      <form className="px-5 pb-5" onSubmit={submit}>
        {error && (
          <div
            className="mb-4 flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-800"
            role="alert"
          >
            {error}
            {agent && (
              <button
                type="button"
                className="shrink-0 text-sm underline"
                onClick={() => {
                  setError('');
                  setRevision((value) => value + 1);
                }}
              >
                重新读取
              </button>
            )}
          </div>
        )}
        <fieldset
          className="grid gap-4 border-0 p-0"
          disabled={saving || Boolean(agent && !version)}
        >
          <label className="grid gap-2 text-sm text-zinc-600">
            名称
            <input
              className={fieldClass}
              name="name"
              required
              maxLength={120}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <label className="grid gap-2 text-sm text-zinc-600">
            简介
            <textarea
              className={fieldClass}
              name="description"
              rows={3}
              maxLength={2000}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          {!agent && (
            <label className="grid gap-2 text-sm text-zinc-600">
              SOUL.md
              <textarea
                className={`${fieldClass} min-h-32 font-mono text-[13px]`}
                name="soul"
                rows={6}
                maxLength={20000}
                value={soul}
                onChange={(event) => setSoul(event.target.value)}
              />
            </label>
          )}
        </fieldset>
        <footer className="mt-4 flex justify-end gap-2">
          <Button variant="outline" disabled={saving} onClick={onCancel}>
            取消
          </Button>
          <Button type="submit" disabled={saving || Boolean(agent && !version)}>
            {saving ? '保存中…' : agent ? '保存' : '创建'}
          </Button>
        </footer>
      </form>
    </Dialog>
  );
}
