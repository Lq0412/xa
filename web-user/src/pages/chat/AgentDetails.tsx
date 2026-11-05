import { useEffect, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
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

/** 默认阅读配置，编辑时只打开对应模块；保存使用不可变版本与乐观锁。 */
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
        }
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
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
      setNotice('已保存');
      setEditing(false);
      onSaved(result.agent);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  return (
    <section className="agent-details-content" aria-label="Agent 配置">
      {onClose && (
        <header className="agent-details-header">
          <span>配置</span>
          <button
            type="button"
            className="icon-button"
            onClick={onClose}
            aria-label="收起 Agent 配置"
          >
            <Icon name="close" />
          </button>
        </header>
      )}
      <div className="agent-details-scroll">
        <div className="details-identity">
          <AgentAvatar agent={agent} large />
          <h2>{agent.name}</h2>
          <span className="muted">
            {agent.is_primary
              ? '主 Agent'
              : agent.visibility === 'public'
                ? '公共 Agent'
                : '私有 Agent'}
          </span>
          {agent.description && <p>{agent.description}</p>}
          {editable && (
            <button type="button" className="button quiet" onClick={onEdit}>
              <Icon name="edit" size={15} />
              编辑基本信息
            </button>
          )}
        </div>
        {error && (
          <div className="error-banner" role="alert">
            {error}
            <button
              type="button"
              className="text-button"
              disabled={saving}
              onClick={() => setRevision((value) => value + 1)}
            >
              重新读取
            </button>
          </div>
        )}
        {!version ? (
          !error && (
            <p className="muted" role="status">
              正在读取配置…
            </p>
          )
        ) : (
          <div className="agent-modules">
            <section className="agent-module">
              <header className="module-heading">
                <h3>SOUL.md</h3>
                <span className="version-label">v{version.version_number}</span>
                {editable && !editing && (
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="编辑 SOUL.md"
                    onClick={() => {
                      setEditing(true);
                      setPreview(false);
                      setNotice('');
                    }}
                  >
                    <Icon name="edit" size={16} />
                  </button>
                )}
              </header>
              {editing ? (
                <div className="soul-editing">
                  <div className="editor-tabs">
                    <button
                      type="button"
                      aria-pressed={!preview}
                      onClick={() => setPreview(false)}
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      aria-pressed={preview}
                      onClick={() => setPreview(true)}
                    >
                      预览
                    </button>
                  </div>
                  {preview ? (
                    <div className="soul-preview">
                      <Markdown>{soul}</Markdown>
                    </div>
                  ) : (
                    <textarea
                      className="soul-editor"
                      aria-label="SOUL.md"
                      maxLength={20000}
                      rows={10}
                      value={soul}
                      disabled={saving}
                      onChange={(event) => setSoul(event.target.value)}
                    />
                  )}
                  <div className="form-actions">
                    <button
                      type="button"
                      className="button quiet"
                      disabled={saving}
                      onClick={() => {
                        setSoul(version.configuration.soul_md);
                        setEditing(false);
                      }}
                    >
                      取消
                    </button>
                    <button
                      type="button"
                      className="button primary"
                      disabled={
                        saving || soul === version.configuration.soul_md
                      }
                      onClick={() => void save('soul_md')}
                    >
                      {saving ? '保存中…' : '保存'}
                    </button>
                  </div>
                </div>
              ) : soul ? (
                <div className="soul-preview">
                  <Markdown>{soul}</Markdown>
                </div>
              ) : (
                <p className="muted">未设置</p>
              )}
            </section>
            <section className="agent-module">
              <header className="module-heading">
                <h3>模型</h3>
              </header>
              {editable ? (
                <>
                  <select
                    aria-label="模型"
                    disabled={saving}
                    value={model}
                    onChange={(event) => {
                      setModel(event.target.value);
                      setNotice('');
                    }}
                  >
                    {version.available_models.map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </select>
                  {model !== version.configuration.model && (
                    <div className="form-actions">
                      <button
                        type="button"
                        className="button primary"
                        disabled={saving}
                        onClick={() => void save('model')}
                      >
                        保存
                      </button>
                    </div>
                  )}
                </>
              ) : (
                <p className="model-value">{model}</p>
              )}
            </section>
            <p className="save-notice" role="status">
              {notice}
            </p>
          </div>
        )}
        {agent.created_at && (
          <p className="created-at">
            创建于{' '}
            <time dateTime={agent.created_at}>
              {new Date(agent.created_at).toLocaleDateString('zh-CN')}
            </time>
          </p>
        )}
      </div>
    </section>
  );
}
