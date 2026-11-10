import {
  PageContainer,
  type ProColumns,
  ProTable,
} from '@ant-design/pro-components';
import { request } from '@umijs/max';
import { Button } from 'antd';
import { useState } from 'react';

type ApprovalRecord = {
  id: string;
  created_at: string;
  user: string;
  conversation_id: string;
  agent_id: string;
  agent: string;
  tool_name: string;
  title: string;
  summary: string;
  status: string;
  wait_ms: number;
  decider: string;
  remember: string;
  run_id: string;
};
const statuses: Record<string, string> = {
  pending: '待确认',
  approved: '已允许',
  rejected: '未执行',
  expired: '未确认',
  cancelled: '已取消',
};
const scopes: Record<string, string> = {
  once: '本次',
  conversation: '本对话',
  agent: '此 Agent',
};
const userWeb = process.env.XA_USER_WEB_URL || 'http://localhost:8002';
function conversationURL(row: ApprovalRecord) {
  return `${userWeb}/chat?agentId=${encodeURIComponent(row.agent_id)}&conversationId=${encodeURIComponent(row.conversation_id)}`;
}

/** 审批列表只提供筛选和详情链接，决定由会话 owner 在用户端完成。 */
export default function ApprovalRecords() {
  const [cursor, setCursor] = useState('');
  const [next, setNext] = useState('');
  const columns: ProColumns<ApprovalRecord>[] = [
    {
      title: '时间',
      dataIndex: 'created_at',
      valueType: 'dateTime',
      search: false,
      width: 152,
    },
    {
      title: '用户',
      dataIndex: 'user',
      search: false,
      width: 152,
      ellipsis: true,
    },
    {
      title: 'Agent',
      dataIndex: 'agent',
      search: false,
      width: 100,
      ellipsis: true,
    },
    {
      title: '工具',
      dataIndex: 'tool_name',
      width: 94,
      render: (_, row) => row.title || row.tool_name,
    },
    {
      title: '摘要',
      dataIndex: 'summary',
      search: false,
      width: 125,
      ellipsis: true,
    },
    { title: '决定', dataIndex: 'status', valueEnum: statuses, width: 74 },
    {
      title: '等待',
      dataIndex: 'wait_ms',
      search: false,
      width: 68,
      render: (_, row) => `${Math.ceil(row.wait_ms / 1000)} 秒`,
    },
    {
      title: '决定人',
      dataIndex: 'decider',
      search: false,
      width: 152,
      ellipsis: true,
    },
    {
      title: '记住范围',
      dataIndex: 'remember',
      search: false,
      width: 85,
      render: (_, row) => scopes[row.remember] || '—',
    },
    {
      title: '会话',
      dataIndex: 'conversation_id',
      search: false,
      width: 87,
      render: (_, row) => (
        <a
          href={conversationURL(row)}
          target="_blank"
          rel="noreferrer"
          title={row.conversation_id}
        >
          {row.conversation_id.slice(5, 13)}
        </a>
      ),
    },
    {
      title: '轮次',
      dataIndex: 'run_id',
      search: false,
      width: 84,
      render: (_, row) => (
        <a
          href={`${conversationURL(row)}#run-${encodeURIComponent(row.run_id)}`}
          target="_blank"
          rel="noreferrer"
          title={row.run_id}
        >
          {row.run_id.slice(4, 12)}
        </a>
      ),
    },
  ];
  return (
    <PageContainer title="审批记录">
      <ProTable<ApprovalRecord>
        rowKey="id"
        columns={columns}
        params={{ cursor }}
        pagination={false}
        scroll={{ x: 1173 }}
        onSubmit={() => setCursor('')}
        onReset={() => setCursor('')}
        request={async (params) => {
          const result = await request<{
            approvals: ApprovalRecord[];
            next_cursor: string;
          }>('/api/v1/admin/approvals', {
            params: {
              status: params.status,
              tool_name: params.tool_name,
              cursor: params.cursor,
              limit: 50,
            },
          });
          setNext(result.next_cursor);
          return { data: result.approvals, success: true };
        }}
        toolBarRender={() => [
          <Button
            key="first"
            type="link"
            disabled={!cursor}
            onClick={() => setCursor('')}
          >
            最新
          </Button>,
          <Button
            key="next"
            type="link"
            disabled={!next}
            onClick={() => setCursor(next)}
          >
            下一页
          </Button>,
        ]}
      />
    </PageContainer>
  );
}
