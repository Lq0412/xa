import { CircleAlert } from 'lucide-react';
import type { ReactNode } from 'react';

/** 页标题和主操作共用一行，让列表保持独立、完整。 */
export function Page({
  title,
  actions,
  children,
}: {
  title: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="page-content">
      <div className="page-heading">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {actions}
      </div>
      {children}
    </div>
  );
}

/** 请求失败在对应页面或表单内展示，读屏会立即播报。 */
export function ErrorNotice({ children }: { children: ReactNode }) {
  return (
    <div
      role="alert"
      className="flex items-start gap-2 rounded-xl bg-red-50 px-3.5 py-3 text-sm text-red-800"
    >
      <CircleAlert aria-hidden="true" size={16} className="mt-0.5 shrink-0" />
      <span>{children}</span>
    </div>
  );
}

/** 独立结果页共用居中的排版。 */
export function ResultPage({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <div className="grid min-h-[60vh] place-items-center px-5 py-12 text-center">
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="text-sm text-zinc-600">{description}</p>}
        {children}
      </div>
    </div>
  );
}
