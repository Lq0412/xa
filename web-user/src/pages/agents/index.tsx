import {
  ArrowRightOutlined,
  PlusOutlined,
  RobotOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { history, Link, useLocation, useModel } from '@umijs/max';
import {
  Alert,
  App,
  Avatar,
  Button,
  Empty,
  Form,
  Input,
  Modal,
  Skeleton,
  Tag,
} from 'antd';
import { useEffect, useState } from 'react';
import {
  type Agent,
  createAgent,
  errorMessage,
  listAgents,
} from '@/services/api';

/** Agent 发现与个人列表使用真实数据；创建后直接进入个人列表。 */
export default function AgentsPage() {
  const { pathname } = useLocation();
  const mine = pathname === '/my-agents';
  const { initialState } = useModel('@@initialState');
  const user = initialState?.currentUser;
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const [revision, setRevision] = useState(0);
  const [creating, setCreating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [form] = Form.useForm();
  const { message } = App.useApp();

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 是保存成功或手动重试时触发重新查询的显式版本号。
  useEffect(() => {
    let disposed = false;
    setLoading(true);
    setError('');
    listAgents(mine)
      .then((result) => {
        if (!disposed)
          setAgents(
            (result.agents ?? []).filter(
              (agent) =>
                !mine ||
                (agent.owner_id === user?.id && agent.visibility === 'private'),
            ),
          );
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
  }, [mine, user?.id, revision]);

  const visible = agents.filter((agent) =>
    `${agent.name} ${agent.description} ${(agent.tags ?? []).join(' ')}`
      .toLocaleLowerCase()
      .includes(query.trim().toLocaleLowerCase()),
  );
  const openCreate = () => {
    if (!user) {
      history.push('/login?redirect=/my-agents');
      return;
    }
    form.resetFields();
    setSaveError('');
    setCreating(true);
  };
  return (
    <div className="page-content agent-directory">
      <header className="page-heading">
        <div>
          <span className="eyebrow">AGENTS</span>
          <h1>{mine ? '我的 Agent' : '发现 Agent'}</h1>
          <p>
            {mine
              ? '为自己的任务创建助手，随时回来继续使用。'
              : '浏览公开的 Agent，找到适合你的助手。'}
          </p>
        </div>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          创建 Agent
        </Button>
      </header>
      <div className="directory-toolbar">
        <div className="directory-tabs">
          <Link to="/explore" className={!mine ? 'active' : ''}>
            公共 Agent
          </Link>
          <Link to="/my-agents" className={mine ? 'active' : ''}>
            我的 Agent
          </Link>
        </div>
        <Input
          aria-label="搜索 Agent"
          placeholder="搜索名称、介绍或标签"
          prefix={<SearchOutlined />}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          allowClear
          className="agent-search"
        />
      </div>
      {error ? (
        <Alert
          type="error"
          showIcon
          title="加载 Agent 失败"
          description={error}
          action={
            <Button onClick={() => setRevision((value) => value + 1)}>
              重试
            </Button>
          }
        />
      ) : loading ? (
        <Skeleton active paragraph={{ rows: 5 }} />
      ) : visible.length ? (
        <div className="agent-grid">
          {visible.map((agent) => (
            <article key={agent.id} className="agent-card">
              <div className="agent-card-top">
                <Avatar
                  shape="square"
                  size={48}
                  src={agent.avatar || undefined}
                  icon={<RobotOutlined />}
                />
                <Tag>{agent.visibility === 'public' ? '公共' : '私有'}</Tag>
              </div>
              <h2>{agent.name}</h2>
              <p>{agent.description || '暂未填写介绍'}</p>
              <div className="agent-tags">
                {(agent.tags ?? []).map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </div>
              <div className="agent-card-bottom">
                <span>
                  {agent.status === 'offline' ? '已停用' : '文本对话待开发'}
                </span>
                <Link to={`/chat?agent=${encodeURIComponent(agent.id)}`}>
                  进入对话 <ArrowRightOutlined />
                </Link>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="directory-empty">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <>
                <h2>
                  {query
                    ? '没有匹配的 Agent'
                    : mine
                      ? '还没有自己的 Agent'
                      : '暂无公共 Agent'}
                </h2>
                <p>
                  {query
                    ? '试试其他名称或标签。'
                    : mine
                      ? '创建一个 Agent，设置名称、介绍和提示词。'
                      : '管理员开放 Agent 后，会显示在这里。你也可以创建自己的 Agent。'}
                </p>
              </>
            }
          >
            <Button onClick={query ? () => setQuery('') : openCreate}>
              {query ? '清除搜索' : '创建 Agent'}
            </Button>
          </Empty>
        </div>
      )}
      <Modal
        title="创建 Agent"
        open={creating}
        onCancel={() => {
          if (!saving) setCreating(false);
        }}
        onOk={() => form.submit()}
        okText="创建"
        cancelText="取消"
        confirmLoading={saving}
        closable={!saving}
        mask={{ closable: !saving }}
        cancelButtonProps={{ disabled: saving }}
      >
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          onFinish={async (values: {
            name: string;
            description?: string;
            prompt?: string;
          }) => {
            setSaving(true);
            setSaveError('');
            try {
              await createAgent(
                values.name.trim(),
                values.description?.trim() || '',
                values.prompt || '',
              );
              setCreating(false);
              message.success('Agent 已创建');
              setRevision((value) => value + 1);
              history.push('/my-agents');
            } catch (cause) {
              setSaveError(errorMessage(cause));
            } finally {
              setSaving(false);
            }
          }}
        >
          <p className="muted">新建 Agent 仅你自己可见。</p>
          {saveError && (
            <Alert
              className="form-alert"
              type="error"
              showIcon
              title={saveError}
            />
          )}
          <Form.Item
            name="name"
            label="名称"
            rules={[
              {
                required: true,
                whitespace: true,
                max: 120,
                message: '请输入 1–120 个字符的名称',
              },
            ]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="description"
            label="介绍"
            rules={[{ max: 2000, message: '介绍最多 2000 个字符' }]}
          >
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item
            name="prompt"
            label="系统提示词"
            rules={[{ max: 20000, message: '提示词最多 20000 个字符' }]}
          >
            <Input.TextArea rows={5} placeholder="描述它的职责和回答方式" />
          </Form.Item>
          <p className="muted">模型、工具及知识库绑定待开发。</p>
        </Form>
      </Modal>
    </div>
  );
}
