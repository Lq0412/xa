import type { ToolPart } from '@/services/chat-parts';

export type PendingApproval = ToolPart & {
  approval: NonNullable<ToolPart['approval']>;
};

/** 预览优先说明实际对象，原始参数只供用户主动查看。 */
export function approvalCopy(part: ToolPart) {
  const title = part.title || part.name;
  const items = part.approval?.items ?? [];
  const summary = part.approval?.summary;
  const forget = part.name === 'memory_forget';
  const subject = items.find((item) =>
    forget ? item.label === '内容' : !/id|编号|命名空间|原因/i.test(item.label),
  );
  const detailItems = items.filter((item) => item !== subject);
  if (forget && subject?.value) {
    const action = `忘掉「${subject.value}」`;
    return {
      title,
      action,
      sentence: `要${action}这条记忆。`,
      subject: subject.value,
      detailItems,
    };
  }
  const action = subject?.value
    ? `${summary || title}：${subject.value}`
    : summary || `使用${title}`;
  return {
    title,
    action,
    sentence: /[。？！]$/.test(action) ? action : `${action}。`,
    subject: '',
    detailItems,
  };
}

/** 收据只复述已记录的决定，兼容缺少范围的旧事件。 */
export function approvalReceipt(part: ToolPart) {
  const { title, action } = approvalCopy(part);
  switch (part.approval?.decision) {
    case 'approved':
      if (part.approval.remember === 'conversation')
        return `已允许，本对话不再询问${title}`;
      if (part.approval.remember === 'agent')
        return `已允许，以后在这个 Agent 上都不再询问${title}`;
      return '已允许本次';
    case 'rejected':
      return `已拒绝${action.replace(/[。？！]$/, '')}`;
    case 'expired':
      return '超时未确认，已视为拒绝';
    case 'cancelled':
      return '已取消';
    default:
      return '';
  }
}

export function approvalCountdown(expiresAt: string, now: number) {
  const remaining = Date.parse(expiresAt) - now;
  if (!Number.isFinite(remaining) || remaining >= 120_000) return '';
  if (remaining <= 0) return '已到期，等待处理';
  const seconds = Math.ceil(remaining / 1000);
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  const duration = [minutes && `${minutes} 分`, rest && `${rest} 秒`]
    .filter(Boolean)
    .join(' ');
  return `将在 ${duration}后视为拒绝`;
}
