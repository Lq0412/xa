import { PlusOutlined } from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ModalForm,
  PageContainer,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Alert, App, Button, Popconfirm, Tag } from 'antd';
import { useRef, useState } from 'react';
import { ApiError } from '@/requestErrorConfig';
import {
  type AgentSaveInput,
  createAgent,
  deleteAgent,
  getAgent,
  getAgentConfig,
  listAgents,
  saveAgent,
  setAgentStatus,
} from '@/services/xa/api';

/** 表单只编辑基础字段；完整配置和乐观锁版本单独保存在 editor 中。 */
type AgentForm = {
  name: string;
  description: string;
  tagsText: string;
  visibility: XA.Agent['visibility'];
  prompt: string;
};
/** 编辑会话绑定读取时的完整版本，避免表单值覆盖其他配置。 */
type Editor = {
  agent?: XA.Agent;
  version?: XA.AgentVersion;
  values: AgentForm;
  readonly: boolean;
  promptIndex: number;
};
/** ProTable 查询条件，对后端返回的可见列表进行本地过滤。 */
type Search = { name?: string; visibility?: string; status?: string };

/** 使用 ProTable 和 ModalForm 提供 Agent 管理；数据全部来自真实后端。 */
export default function AgentsPage() {
  const actionRef = useRef<ActionType>(null);
  const { canAdmin, canManageAgent } = useAccess();
  const { message } = App.useApp();
  const [editor, setEditor] = useState<Editor>();
  const [formError, setFormError] = useState('');
  const [listError, setListError] = useState('');
  const [pendingId, setPendingId] = useState<string>();
  const [saving, setSaving] = useState(false);

  const openCreate = () => {
    setFormError('');
    setEditor({
      readonly: false,
      promptIndex: -1,
      values: {
        name: '',
        description: '',
        tagsText: '',
        visibility: canAdmin ? 'public' : 'private',
        prompt: '',
      },
    });
  };

  const openEditor = async (row: XA.Agent) => {
    setPendingId(row.id);
    setFormError('');
    try {
      const [agent, version] = await Promise.all([
        getAgent(row.id),
        getAgentConfig(row.id),
      ]);
      const prompts = version.configuration.prompts ?? [];
      const enabled = prompts.findIndex((item) => item.enabled);
      const promptIndex = enabled >= 0 ? enabled : prompts.length ? 0 : -1;
      setEditor({
        agent,
        version,
        readonly: !canManageAgent(agent),
        promptIndex,
        values: {
          name: agent.name,
          description: agent.description,
          tagsText: (agent.tags ?? []).join(', '),
          visibility: agent.visibility,
          prompt: prompts[promptIndex]?.content ?? '',
        },
      });
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      message.error(cause.message);
    } finally {
      setPendingId(undefined);
    }
  };

  // 状态写入成功后才刷新，失败会保留原行并显示服务端原因。
  const changeStatus = async (agent: XA.Agent, remove = false) => {
    setPendingId(agent.id);
    try {
      if (remove) await deleteAgent(agent.id);
      else await setAgentStatus(agent.id, agent.status !== 'active');
      message.success(
        remove
          ? 'Agent 已删除'
          : agent.status === 'active'
            ? 'Agent 已停用'
            : 'Agent 已启用',
      );
      await actionRef.current?.reload();
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      message.error(cause.message);
    } finally {
      setPendingId(undefined);
    }
  };

  const columns: ProColumns<XA.Agent>[] = [
    {
      title: '名称',
      dataIndex: 'name',
      width: 200,
      render: (_, agent) => (
        <Button
          type="link"
          disabled={Boolean(pendingId)}
          onClick={() => void openEditor(agent)}
        >
          {agent.name}
          {agent.is_default && <Tag color="blue">默认</Tag>}
        </Button>
      ),
    },
    { title: '描述', dataIndex: 'description', ellipsis: true, search: false },
    {
      title: '可见范围',
      dataIndex: 'visibility',
      width: 110,
      valueEnum: {
        public: { text: '公共', status: 'Success' },
        private: { text: '私有', status: 'Processing' },
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      valueEnum: {
        active: { text: '已启用', status: 'Success' },
        offline: { text: '已停用', status: 'Default' },
      },
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      valueType: 'dateTime',
      width: 180,
      search: false,
    },
    {
      title: '操作',
      valueType: 'option',
      width: 200,
      render: (_, agent) => {
        const disabled = Boolean(pendingId);
        if (!canManageAgent(agent))
          return [
            <Button
              key="view"
              type="link"
              disabled={disabled}
              onClick={() => void openEditor(agent)}
            >
              查看
            </Button>,
          ];
        return [
          <Button
            key="edit"
            type="link"
            disabled={disabled}
            onClick={() => void openEditor(agent)}
          >
            编辑
          </Button>,
          <Button
            key="toggle"
            type="link"
            disabled={disabled}
            loading={pendingId === agent.id}
            onClick={() => void changeStatus(agent)}
          >
            {agent.status === 'active' ? '停用' : '启用'}
          </Button>,
          <Popconfirm
            key="delete"
            title={`删除「${agent.name}」？`}
            description="删除后将从 Agent 列表移除。"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            disabled={disabled}
            onConfirm={() => changeStatus(agent, true)}
          >
            <Button type="link" danger disabled={disabled}>
              删除
            </Button>
          </Popconfirm>,
        ];
      },
    },
  ];

  return (
    <PageContainer
      title="Agent 管理"
      content="管理 Agent 的基础信息、系统提示词与可用状态。"
    >
      {listError && (
        <Alert
          type="error"
          title={listError}
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}
      <ProTable<XA.Agent, Search>
        headerTitle="Agent 列表"
        form={{ name: 'agent-search' }}
        actionRef={actionRef}
        rowKey="id"
        columns={columns}
        scroll={{ x: 980 }}
        request={async (params) => {
          setListError('');
          const result = await listAgents();
          // 当前接口返回完整可见列表，筛选和分页在客户端执行，不伪造服务端分页参数。
          const name = params.name?.trim().toLocaleLowerCase();
          const data = (result.agents ?? []).filter(
            (agent) =>
              (!name || agent.name.toLocaleLowerCase().includes(name)) &&
              (!params.visibility || agent.visibility === params.visibility) &&
              (!params.status || agent.status === params.status),
          );
          return { data, success: true, total: data.length };
        }}
        onRequestError={(cause) => {
          if (!(cause instanceof ApiError)) throw cause;
          setListError(cause.message);
        }}
        search={{ labelWidth: 'auto', defaultCollapsed: false }}
        pagination={{ defaultPageSize: 10, showSizeChanger: true }}
        toolBarRender={() => [
          <Button
            key="create"
            type="primary"
            icon={<PlusOutlined />}
            disabled={Boolean(pendingId)}
            onClick={openCreate}
          >
            新建 Agent
          </Button>,
        ]}
      />
      {editor && (
        <ModalForm<AgentForm>
          name="agent-editor"
          title={
            editor.readonly
              ? '查看 Agent'
              : editor.agent
                ? '编辑 Agent'
                : '新建 Agent'
          }
          open
          initialValues={editor.values}
          disabled={editor.readonly}
          width={680}
          preserve={false}
          modalProps={{
            destroyOnHidden: true,
            maskClosable: !saving,
            closable: !saving,
            keyboard: !saving,
          }}
          onOpenChange={(open) => {
            if (!open && !saving) setEditor(undefined);
          }}
          submitter={
            editor.readonly
              ? false
              : {
                  searchConfig: { submitText: '保存', resetText: '取消' },
                  resetButtonProps: { disabled: saving },
                }
          }
          onFinish={async (values) => {
            setSaving(true);
            setFormError('');
            try {
              const data: AgentSaveInput = {
                name: values.name.trim(),
                avatar: editor.agent?.avatar ?? '',
                description: (values.description ?? '').trim(),
                tags: (values.tagsText ?? '')
                  .split(/[,，]/)
                  .map((tag) => tag.trim())
                  .filter(Boolean),
                visibility: values.visibility,
                configuration: updatePrompt(
                  editor.version?.configuration ?? { prompts: [] },
                  values.prompt ?? '',
                  editor.promptIndex,
                ),
              };
              if (editor.agent && editor.version)
                await saveAgent(editor.agent.id, editor.version.id, data);
              else await createAgent(data);
              message.success(
                editor.agent ? 'Agent 配置已保存' : 'Agent 已创建',
              );
              setEditor(undefined);
              await actionRef.current?.reload();
              return true;
            } catch (cause) {
              if (!(cause instanceof ApiError)) throw cause;
              setFormError(
                cause.status === 409
                  ? '配置已被更新。请关闭后重新打开，核对最新内容再保存。'
                  : cause.message,
              );
              return false;
            } finally {
              setSaving(false);
            }
          }}
        >
          {formError && (
            <Alert
              type="error"
              title={formError}
              showIcon
              style={{ marginBottom: 24 }}
            />
          )}
          <ProFormText
            name="name"
            label="名称"
            rules={[
              {
                required: true,
                whitespace: true,
                message: '请输入 Agent 名称',
              },
              { max: 120, message: '名称不能超过 120 个字符' },
            ]}
          />
          <ProFormSelect
            name="visibility"
            label="可见范围"
            disabled={!canAdmin || Boolean(editor.agent)}
            options={[
              { label: '私有', value: 'private' },
              { label: '公共', value: 'public' },
            ]}
          />
          <ProFormTextArea
            name="description"
            label="描述"
            fieldProps={{ rows: 3, maxLength: 2000, showCount: true }}
          />
          <ProFormText
            name="tagsText"
            label="标签"
            extra="多个标签用逗号分隔"
          />
          <ProFormTextArea
            name="prompt"
            label="系统提示词"
            fieldProps={{ rows: 8, showCount: true }}
            rules={[{ max: 20000, message: '提示词不能超过 20000 个字符' }]}
          />
        </ModalForm>
      )}
    </PageContainer>
  );
}

/** 只修改当前展示的提示词，其他提示词、模型绑定和策略按原配置保留。 */
function updatePrompt(
  configuration: XA.Configuration,
  content: string,
  index: number,
): XA.Configuration {
  const prompts = [...(configuration.prompts ?? [])];
  const trimmed = content.trim();
  if (index >= 0) {
    if (trimmed) prompts[index] = { ...prompts[index], content: trimmed };
    else prompts.splice(index, 1);
  } else if (trimmed) {
    prompts.push({
      name: 'system',
      content: trimmed,
      enabled: true,
      order: 0,
      variables: [],
      scope: 'agent',
    });
  }
  return { ...configuration, prompts };
}
