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
      const part = next.find(
        (p): p is Extract<MessagePart, { type: 'reasoning' }> =>
          p.type === 'reasoning' && p.item_id === itemId,
      );
      if (part) {
        part.status = 'completed';
        part.truncated = Boolean(ev.truncated);
      }
      break;
    }
    case 'tool.preparing':
    case 'tool.called':
    case 'tool.started':
    case 'tool.completed': {
      const callId = ev.call_id as string | undefined;
      const index = ev.index as number | undefined;
      let part = next.find(
        (p): p is Extract<MessagePart, { type: 'tool' }> =>
          p.type === 'tool' &&
          ((callId && p.call_id === callId) ||
            (!callId && p.index === index && !p.call_id)),
      );
      if (!part) {
        part = {
          type: 'tool',
          name: String(ev.name ?? ''),
          title: String(ev.title ?? ev.name ?? '工具'),
          status: 'preparing',
          call_id: callId,
          index,
        };
        next.push(part);
      }
      if (ev.type === 'tool.preparing') part.status = 'preparing';
      if (ev.type === 'tool.called') {
        part.call_id = String(ev.call_id ?? part.call_id);
        part.status = String(ev.status) === 'rejected' ? 'rejected' : 'pending';
        if (ev.args_preview) part.args_preview = String(ev.args_preview);
      }
      if (ev.type === 'tool.started') part.status = 'running';
      if (ev.type === 'tool.completed') {
        part.status = String(ev.status ?? 'succeeded') as ToolPartStatus;
        if (ev.summary) part.summary = String(ev.summary);
        if (Array.isArray(ev.items)) part.items = ev.items as ToolPart['items'];
        if (ev.duration_ms) part.duration_ms = Number(ev.duration_ms);
      }
      break;
    }
  }
  return next;
}

type ToolPart = Extract<MessagePart, { type: 'tool' }>;
type ToolPartStatus = ToolPart['status'];
