import { type FormEvent, useEffect, useState } from 'react';
import {
  type Agent,
  type AgentVersion,
  ApiError,
  createAgent,
  errorMessage,
  getAgentConfig,
  saveAgent,
} from '@/services/api';
import { Dialog } from './Dialog';

type Props = {
  agent?: Agent;
  onCancel: () => void;
  onSaved: (agent: Agent) => void;
};

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
      <form className="editor-form" onSubmit={submit}>
        {error && (
          <div className="error-banner" role="alert">
            {error}
            {agent && (
              <button
                type="button"
                className="text-button"
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
        <fieldset disabled={saving || Boolean(agent && !version)}>
          <label>
            名称
            <input
              name="name"
              required
              maxLength={120}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <label>
            简介
            <textarea
              name="description"
              rows={3}
              maxLength={2000}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          {!agent && (
            <label>
              SOUL.md
              <textarea
                name="soul"
                className="soul-editor"
                rows={6}
                maxLength={20000}
                value={soul}
                onChange={(event) => setSoul(event.target.value)}
              />
            </label>
          )}
        </fieldset>
        <footer className="form-actions">
          <button
            type="button"
            className="button"
            disabled={saving}
            onClick={onCancel}
          >
            取消
          </button>
          <button
            type="submit"
            className="button primary"
            disabled={saving || Boolean(agent && !version)}
          >
            {saving ? '保存中…' : agent ? '保存' : '创建'}
          </button>
        </footer>
      </form>
    </Dialog>
  );
}
