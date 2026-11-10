import type { MessagePart, TextPart } from '@/services/chat-parts';

export type AssistantLayout = {
  process: MessagePart[];
  text: TextPart[];
};

/** 过程轨（思考、工具）与回答正文拆分。 */
export function splitAssistantParts(parts: MessagePart[]): AssistantLayout {
  const process: MessagePart[] = [];
  const text: TextPart[] = [];
  for (const part of parts) {
    if (part.type === 'text' && part.text.trim()) text.push(part);
    else if (part.type === 'reasoning' || part.type === 'tool')
      process.push(part);
  }
  return { process, text };
}

/**
 * 历史 parts 偶尔会出现正文在思考/工具之前；把落在第一段正文之后的 process part 整体上移。
 */
export function reorderReasoningBeforeText(
  parts: MessagePart[],
): MessagePart[] {
  const firstTextIdx = parts.findIndex(
    (p) => p.type === 'text' && p.text.trim().length > 0,
  );
  if (firstTextIdx < 0) return parts;
  const head = parts.slice(0, firstTextIdx);
  const tail = parts.slice(firstTextIdx);
  const misplaced = tail.filter(
    (p) => p.type === 'reasoning' || p.type === 'tool',
  );
  if (misplaced.length === 0) return parts;
  const tailKeep = tail.filter(
    (p) => p.type !== 'reasoning' && p.type !== 'tool',
  );
  return [...head, ...misplaced, ...tailKeep];
}

export function partIndexInAll(
  parts: MessagePart[],
  target: MessagePart,
): number {
  return parts.findIndex((p) => {
    if (p.type !== target.type) return false;
    if (p.type === 'reasoning' && target.type === 'reasoning')
      return p.item_id === target.item_id;
    if (p.type === 'tool' && target.type === 'tool') {
      const a = p as Extract<MessagePart, { type: 'tool' }>;
      const b = target as Extract<MessagePart, { type: 'tool' }>;
      if (a.call_id && b.call_id) return a.call_id === b.call_id;
      return a.name === b.name && a.index === b.index;
    }
    return p === target;
  });
}

/** 后续已有工具或正文时，把仍在 streaming 的思考视为已结束（防御 reasoning.completed 迟到）。 */
export function reasoningHasLaterActivity(
  parts: MessagePart[],
  itemId: string,
): boolean {
  const idx = parts.findIndex(
    (p) => p.type === 'reasoning' && p.item_id === itemId,
  );
  if (idx < 0) return false;
  return parts.slice(idx + 1).some((p) => {
    if (p.type === 'tool') return true;
    if (p.type === 'text' && p.text.trim().length > 0) return true;
    if (p.type === 'reasoning' && p.item_id !== itemId) return true;
    return false;
  });
}
