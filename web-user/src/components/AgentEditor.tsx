import { Alert, Form, Input, Modal, Skeleton } from 'antd';
import { useEffect, useState } from 'react';
import {
  type Agent,
  type AgentVersion,
  ApiError,
  createAgent,
  errorMessage,
  getAgent,
  getAgentConfig,
  saveAgent,
} from '@/services/api';

type Values = { name: string; description?: string; prompt?: string };
type Props = {
  agent?: Agent;
  onCancel: () => void;
  onSaved: (agent: Agent) => void;
};

/** 创建和编辑共用一份表单；编辑读取完整版本并携带版本号保存。 */
export function AgentEditor({ agent, onCancel, onSaved }: Props) {
  const [form] = Form.useForm<Values>();
  const [snapshot, setSnapshot] = useState<{
    agent: Agent;
    version: AgentVersion;
    promptIndex: number;
  }>();
  const [loading, setLoading] = useState(Boolean(agent));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 仅由重新加载按钮递增。
  useEffect(() => {
    if (!agent) return;
    const controller = new AbortController();
    setLoading(true);
    setError('');
    Promise.all([
      getAgent(agent.id, controller.signal),
      getAgentConfig(agent.id, controller.signal),
    ])
      .then(([current, version]) => {
        if (controller.signal.aborted) return;
        if (current.current_version_id !== version.id)
          throw new ApiError(409, '配置已更新，请重新加载');
        const prompts = version.configuration.prompts ?? [];
        const enabled = prompts.findIndex((slot) => slot.enabled);
        const promptIndex = enabled >= 0 ? enabled : prompts.length ? 0 : -1;
        setSnapshot({ agent: current, version, promptIndex });
        form.setFieldsValue({
          name: current.name,
          description: current.description,
          prompt: prompts[promptIndex]?.content || '',
        });
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [agent, form, revision]);
  return (
    <Modal
      open
      title={agent ? '编辑 Agent' : '新建 Agent'}
      onCancel={onCancel}
      onOk={() => form.submit()}
      okText={agent ? '保存' : '创建'}
      cancelText="取消"
      confirmLoading={saving}
      closable={!saving}
      keyboard={!saving}
      mask={{ closable: !saving }}
      cancelButtonProps={{ disabled: saving }}
      okButtonProps={{ disabled: loading || Boolean(agent && !snapshot) }}
    >
      {error && (
        <Alert
          className="form-alert"
          type="error"
          title={error}
          showIcon
          action={
            agent && (
              <button
                type="button"
                className="text-button"
                disabled={saving}
                onClick={() => setRevision((value) => value + 1)}
              >
                重新加载
              </button>
            )
          }
        />
      )}
      {loading ? (
        <Skeleton active />
      ) : (
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          disabled={saving || Boolean(agent && !snapshot)}
          onFinish={async (values) => {
            if (saving) return;
            setSaving(true);
            setError('');
            try {
              if (agent && !snapshot)
                throw new ApiError(409, '请先重新加载 Agent 配置');
              const input = {
                name: values.name.trim(),
                description: values.description?.trim() || '',
                prompt: values.prompt || '',
              };
              const result = snapshot
                ? await saveAgent(
                    snapshot.agent,
                    snapshot.version,
                    input,
                    snapshot.promptIndex,
                  )
                : await createAgent(
                    input.name,
                    input.description,
                    input.prompt,
                  );
              onSaved(result.agent);
            } catch (cause) {
              setError(errorMessage(cause));
            } finally {
              setSaving(false);
            }
          }}
        >
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
            <Input autoFocus />
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
            <Input.TextArea rows={6} />
          </Form.Item>
        </Form>
      )}
    </Modal>
  );
}
