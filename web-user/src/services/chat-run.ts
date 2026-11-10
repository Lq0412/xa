import type { MessagePart } from './chat-parts';

export type RunStreamEvent = {
  seq: number;
  type: string;
  [key: string]: unknown;
};

/** 将 v2 事件归约为 MessagePart 列表（幂等）。 */
export function applyRunEvent(
  parts: MessagePart[],
  ev: RunStreamEvent,
): MessagePart[] {
  const next = parts.map((p) =>
    p.type === 'text' || p.type === 'reasoning'
      ? { ...p, ...(p.type === 'text' ? { text: p.text } : { text: p.text }) }
      : { ...p },
  ) as MessagePart[];
  const pushText = (delta: string) => {
    const last = next[next.length - 1];
    if (last?.type === 'text') {
      (last as { type: 'text'; text: string }).text += delta;
    } else {
      next.push({ type: 'text', text: delta });
    }
  };
  switch (ev.type) {
    case 'message.delta':
      pushText(String(ev.delta ?? ''));
      break;
    case 'reasoning.delta': {
      const itemId = String(ev.item_id ?? 'reason');
      let idx = next.findIndex(
        (p) => p.type === 'reasoning' && p.item_id === itemId,
      );
      if (idx < 0) {
        next.push({
          type: 'reasoning',
          item_id: itemId,
          text: '',
          status: 'streaming',
        });
        idx = next.length - 1;
      }
      const part = next[idx] as Extract<MessagePart, { type: 'reasoning' }>;
      part.text += String(ev.delta ?? '');
      break;
    }
    case 'reasoning.completed': {
      const itemId = String(ev.item_id ?? '');
      let part = next.find(
        (p): p is Extract<MessagePart, { type: 'reasoning' }> =>
          p.type === 'reasoning' && (itemId === '' || p.item_id === itemId),
      );
      if (!part) {
        for (let i = next.length - 1; i >= 0; i--) {
          if (next[i].type === 'reasoning') {
            part = next[i] as Extract<MessagePart, { type: 'reasoning' }>;
            break;
          }
        }
      }
      if (part) {
        part.status = 'completed';
        part.truncated = Boolean(ev.truncated);
        if (ev.duration_ms != null) part.duration_ms = Number(ev.duration_ms);
      }
      break;
    }
    case 'tool.preparing':
    case 'tool.called':
    case 'tool.started':
    case 'tool.completed':
    case 'approval.required':
    case 'approval.resolved': {
      const callId = ev.call_id as string | undefined;
      const index = ev.index as number | undefined;
      const name = String(ev.name ?? '');
      let partIdx = next.findIndex((p) => {
        if (p.type !== 'tool') return false;
        const tool = p as Extract<MessagePart, { type: 'tool' }>;
        if (callId && tool.call_id === callId) return true;
        if (!callId && index != null && tool.index === index) return true;
        if (name && tool.name === name && !tool.call_id) return true;
        return false;
      });
      let part: Extract<MessagePart, { type: 'tool' }>;
      if (partIdx >= 0) {
        part = next[partIdx] as Extract<MessagePart, { type: 'tool' }>;
      } else {
        part = {
          type: 'tool',
          name,
          title: String(ev.title ?? ev.name ?? '工具'),
          status: 'preparing',
          call_id: callId,
          index,
        };
        next.push(part);
        partIdx = next.length - 1;
      }
      if (ev.type === 'tool.called' && callId) part.call_id = callId;
      if (ev.type === 'tool.started' && callId) part.call_id = callId;
      if (ev.type === 'tool.preparing') part.status = 'preparing';
      if (ev.type === 'tool.called') {
        part.status = String(ev.status) === 'rejected' ? 'rejected' : 'pending';
        if (ev.args_preview) part.args_preview = String(ev.args_preview);
      }
      if (ev.type === 'tool.started') part.status = 'running';
      if (ev.type === 'approval.required') {
        part.status = 'awaiting_approval';
        part.call_id = callId;
        part.approval = {
          approval_id: String(ev.approval_id),
          expires_at: String(ev.expires_at),
          summary: String(ev.summary ?? ''),
          items: ev.items as ToolPart['items'],
          can_remember_agent: Boolean(ev.can_remember_agent),
        };
        if (ev.args_preview) part.args_preview = String(ev.args_preview);
      }
      if (ev.type === 'approval.resolved') {
        part.approval = {
          approval_id: String(ev.approval_id),
          expires_at: part.approval?.expires_at ?? '',
          ...part.approval,
          decision: ev.decision as NonNullable<
            ToolPart['approval']
          >['decision'],
          remember: ev.remember as NonNullable<
            ToolPart['approval']
          >['remember'],
        };
        part.status =
          ev.decision === 'approved'
            ? 'pending'
            : ev.decision === 'cancelled'
              ? 'cancelled'
              : 'rejected';
      }
      if (ev.type === 'tool.completed') {
        part.status = String(ev.status ?? 'succeeded') as ToolPartStatus;
        if (ev.summary) part.summary = String(ev.summary);
        if (Array.isArray(ev.items)) part.items = ev.items as ToolPart['items'];
        if (ev.duration_ms) part.duration_ms = Number(ev.duration_ms);
        if (ev.error_code) part.error_code = String(ev.error_code);
      }
      next[partIdx] = part;
      break;
    }
  }
  for (let i = 0; i < next.length; i++) {
    const p = next[i];
    if (p.type !== 'reasoning' || p.status !== 'streaming') continue;
    const later = next.slice(i + 1).some((part) => {
      if (part.type === 'tool') return true;
      if (part.type === 'text' && part.text.trim()) return true;
      if (part.type === 'reasoning' && part.item_id !== p.item_id) return true;
      return false;
    });
    if (later) p.status = 'completed';
  }
  return next;
}

type ToolPart = Extract<MessagePart, { type: 'tool' }>;
type ToolPartStatus = ToolPart['status'];
