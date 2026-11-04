import {
  ArrowUpOutlined,
  AudioOutlined,
  CloseOutlined,
  InfoCircleOutlined,
  MenuOutlined,
  PlusOutlined,
  RobotOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { Link, useLocation } from '@umijs/max';
import { Alert, Avatar, Button, Input, Spin, Tabs, Tag, Tooltip } from 'antd';
import { useEffect, useState } from 'react';
import { type Agent, errorMessage, getAgent } from '@/services/api';

/** 对话页先提供会话栏、消息区和详情栏；运行接口接入前禁止发送假消息。 */
export default function ChatPage() {
  const { search } = useLocation();
  const agentId = new URLSearchParams(search).get('agent');
  const [agent, setAgent] = useState<Agent>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [details, setDetails] = useState(
    () => window.matchMedia('(min-width: 1201px)').matches,
  );
  const [sessionsOpen, setSessionsOpen] = useState(false);
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 使用户点击重试时重新读取同一个 Agent。
  useEffect(() => {
    let disposed = false;
    setAgent(undefined);
    setError('');
    setLoading(Boolean(agentId));
    if (agentId)
      getAgent(agentId)
        .then((result) => {
          if (!disposed) setAgent(result);
        })
        .catch((cause) => {
          if (!disposed) setError(errorMessage(cause));
        })
        .finally(() => {
          if (!disposed) setLoading(false);
        });
    return () => {
      disposed = true;
    };
  }, [agentId, revision]);

  return (
    <div className="chat-layout">
      <aside
        className={`conversation-sidebar ${sessionsOpen ? 'is-open' : ''}`}
        aria-label="会话列表"
      >
        <div className="conversation-search">
          <Input
            prefix={<SearchOutlined />}
            placeholder="搜索会话"
            aria-label="搜索会话"
            disabled
          />
          <Button
            type="text"
            className="mobile-only"
            icon={<CloseOutlined />}
            aria-label="关闭会话列表"
            onClick={() => setSessionsOpen(false)}
          />
        </div>
        <Link
          className="new-conversation"
          to="/chat"
          onClick={() => setSessionsOpen(false)}
        >
          <PlusOutlined /> 新对话
        </Link>
        <div className="sidebar-label">
          最近会话 <span>待开发</span>
        </div>
        <p className="sidebar-note">会话历史接入后，将在这里显示。</p>
        <Link className="sidebar-explore" to="/explore">
          浏览全部 Agent →
        </Link>
      </aside>
      <section className="conversation-main" aria-label="对话区域">
        <header className="conversation-header">
          <div className="conversation-identity">
            <Button
              type="text"
              className="mobile-only"
              icon={<MenuOutlined />}
              aria-label="打开会话列表"
              onClick={() => setSessionsOpen(true)}
            />
            <Avatar
              size={30}
              icon={<RobotOutlined />}
              src={agent?.avatar || undefined}
            />
            <span>{agent?.name || '新对话'}</span>
            <Tag>待开发</Tag>
          </div>
          <Tooltip title="Agent 详情">
            <Button
              type="text"
              icon={<InfoCircleOutlined />}
              aria-label="Agent 详情"
              aria-expanded={details}
              onClick={() => setDetails((value) => !value)}
            />
          </Tooltip>
        </header>
        <div className="conversation-body">
          {loading ? (
            <Spin />
          ) : error ? (
            <Alert
              type="error"
              title="无法读取 Agent"
              description={error}
              showIcon
              action={
                <Button onClick={() => setRevision((value) => value + 1)}>
                  重试
                </Button>
              }
            />
          ) : (
            <div className="conversation-welcome">
              <Avatar
                size={64}
                shape="square"
                icon={<RobotOutlined />}
                src={agent?.avatar || undefined}
              />
              <h1>{agent ? agent.name : '选择一位 Agent'}</h1>
              <p>
                {agent
                  ? agent.description || '这个 Agent 暂未填写介绍。'
                  : '从公共 Agent 中选择，或创建自己的助手。'}
              </p>
              <Link to="/explore">{agent ? '切换 Agent' : '浏览 Agent'} →</Link>
            </div>
          )}
        </div>
        <div className="composer-area">
          <div className="message-composer">
            <Input.TextArea
              aria-label="消息"
              disabled
              autoSize={{ minRows: 1, maxRows: 4 }}
              placeholder={
                agent
                  ? '文本对话待开发，暂不能发送消息'
                  : '选择 Agent 后开始对话'
              }
            />
            <div className="composer-controls">
              <Tooltip title="实时语音待开发">
                <Button
                  type="text"
                  disabled
                  icon={<AudioOutlined />}
                  aria-label="语音对话"
                />
              </Tooltip>
              <Button
                type="primary"
                shape="circle"
                icon={<ArrowUpOutlined />}
                disabled
                aria-label="发送消息"
              />
            </div>
          </div>
          <p className="composer-note">文本与实时语音对话待开发</p>
        </div>
      </section>
      {details && (
        <aside className="agent-details" aria-label="Agent 资料">
          <Button
            className="details-close"
            type="text"
            icon={<CloseOutlined />}
            aria-label="关闭 Agent 详情"
            onClick={() => setDetails(false)}
          />
          <div className="details-identity">
            <Avatar
              size={88}
              src={agent?.avatar || undefined}
              icon={<RobotOutlined />}
            />
            <h2>{agent?.name || 'Agent 详情'}</h2>
            <span className="muted">
              {agent
                ? agent.visibility === 'public'
                  ? '公共 Agent'
                  : '私有 Agent'
                : '选择 Agent 后查看资料'}
            </span>
          </div>
          <Tabs
            defaultActiveKey="about"
            centered
            items={[
              {
                key: 'about',
                label: '简介',
                children: (
                  <div className="details-section">
                    <p>
                      {agent?.description ||
                        '这里展示所选 Agent 的介绍和标签。'}
                    </p>
                    <div className="agent-tags">
                      {(agent?.tags ?? []).map((tag) => (
                        <Tag key={tag}>{tag}</Tag>
                      ))}
                    </div>
                    {agent?.status === 'offline' && (
                      <Alert type="warning" title="此 Agent 已停用" showIcon />
                    )}
                  </div>
                ),
              },
              {
                key: 'capabilities',
                label: '能力',
                children: (
                  <div className="details-section">
                    {['文本对话', '实时语音', '工具与知识库', '记忆与评价'].map(
                      (name) => (
                        <div className="capability-row" key={name}>
                          <span>{name}</span>
                          <Tag>待开发</Tag>
                        </div>
                      ),
                    )}
                  </div>
                ),
              },
            ]}
          />
        </aside>
      )}
    </div>
  );
}
