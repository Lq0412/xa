import { CloseOutlined, EditOutlined, RobotOutlined } from '@ant-design/icons';
import { Alert, Avatar, Button, Skeleton, Tag } from 'antd';
import { useEffect, useState } from 'react';
import {
  type Agent,
  type AgentVersion,
  errorMessage,
  getAgentConfig,
} from '@/services/api';

type Props = {
  agent: Agent;
  userID?: string;
  onEdit: () => void;
  onClose: () => void;
};

/** 详情仅显示后端真实字段，公共 Agent 始终只读；配置失败单独呈现。 */
export function AgentDetails({ agent, userID, onEdit, onClose }: Props) {
  const [version, setVersion] = useState<AgentVersion>();
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: version ID 和 revision 控制编辑保存及重试后的读取。
  useEffect(() => {
    const controller = new AbortController();
    setVersion(undefined);
    setError('');
    getAgentConfig(agent.id, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setVersion(result);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agent.id, agent.current_version_id, revision]);
  const prompts = [...(version?.configuration.prompts ?? [])]
    .sort((a, b) => a.order - b.order)
    .filter((slot) => slot.content.trim());
  return (
    <section className="agent-details-content" aria-label="Agent 资料">
      <header className="agent-details-header">
        <strong>Agent 详情</strong>
        <Button
          type="text"
          icon={<CloseOutlined />}
          aria-label="收起 Agent 详情"
          onClick={onClose}
        />
      </header>
      <div className="agent-details-scroll">
        <div className="details-identity">
          <Avatar
            size={64}
            src={agent.avatar || undefined}
            icon={<RobotOutlined />}
          />
          <h2>{agent.name}</h2>
          <Tag>{agent.visibility === 'public' ? '公共' : '私有'}</Tag>
          {agent.description && <p>{agent.description}</p>}
          {agent.visibility === 'private' &&
            agent.owner_type === 'user' &&
            agent.owner_id === userID && (
              <Button icon={<EditOutlined />} onClick={onEdit}>
                编辑
              </Button>
            )}
        </div>
        <dl className="agent-details-fields">
          {version?.runtime_model && (
            <div>
              <dt>模型</dt>
              <dd>{version.runtime_model}</dd>
            </div>
          )}
          {agent.created_at && (
            <div>
              <dt>创建时间</dt>
              <dd>
                <time dateTime={agent.created_at}>
                  {new Date(agent.created_at).toLocaleString('zh-CN')}
                </time>
              </dd>
            </div>
          )}
        </dl>
        {error ? (
          <Alert
            type="error"
            title={error}
            action={
              <Button
                size="small"
                onClick={() => setRevision((value) => value + 1)}
              >
                重试
              </Button>
            }
          />
        ) : !version ? (
          <Skeleton active paragraph={{ rows: 3 }} />
        ) : (
          prompts.length > 0 && (
            <section className="agent-prompts">
              <h3>系统提示词</h3>
              {prompts.map((slot, index) => (
                // biome-ignore lint/suspicious/noArrayIndexKey: 提示词是不可变版本内的静态片段，没有独立 ID，也不在详情中排序或编辑。
                <div key={`${slot.name}-${index}`}>
                  {(prompts.length > 1 || !slot.enabled) && (
                    <div className="prompt-label">
                      {slot.name}
                      {!slot.enabled && ' · 未启用'}
                    </div>
                  )}
                  <pre>{slot.content}</pre>
                </div>
              ))}
            </section>
          )
        )}
      </div>
    </section>
  );
}
