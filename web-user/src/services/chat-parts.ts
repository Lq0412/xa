export type ReasoningPart = {
  type: 'reasoning';
  item_id: string;
  text: string;
  status: 'streaming' | 'completed' | 'stopped';
  truncated?: boolean;
  duration_ms?: number;
};

export type ToolPart = {
  type: 'tool';
  call_id?: string;
  index?: number;
  name: string;
  title: string;
  status:
    | 'preparing'
    | 'pending'
    | 'awaiting_approval'
    | 'running'
    | 'succeeded'
    | 'failed'
    | 'rejected'
    | 'cancelled'
    | 'unknown';
  args_preview?: string;
  summary?: string;
  items?: { label: string; value: string }[];
  error_code?: string;
  duration_ms?: number;
  approval?: ApprovalInfo;
};

/** 审批展示信息随事件和历史消息保存，决定绑定单个 call_id。 */
export type ApprovalInfo = {
  approval_id: string;
  expires_at: string;
  summary?: string;
  items?: { label: string; value: string }[];
  can_remember_agent?: boolean;
  decision?: 'approved' | 'rejected' | 'expired' | 'cancelled';
  remember?: 'once' | 'conversation' | 'agent';
};

export type TextPart = { type: 'text'; text: string };

export type MessagePart = TextPart | ReasoningPart | ToolPart;
