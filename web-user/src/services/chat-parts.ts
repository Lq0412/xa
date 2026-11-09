export type ReasoningPart = {
  type: 'reasoning';
  item_id: string;
  text: string;
  status: 'streaming' | 'completed';
  truncated?: boolean;
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
};

export type TextPart = { type: 'text'; text: string };

export type MessagePart = TextPart | ReasoningPart | ToolPart;
