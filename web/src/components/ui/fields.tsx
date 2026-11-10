import { ChevronDown } from 'lucide-react';
import type { ComponentProps } from 'react';
import { cn } from '@/lib/utils';

const fieldClass =
  'w-full rounded-xl border border-zinc-200 bg-white px-3 text-sm text-zinc-900 placeholder:text-zinc-500 disabled:bg-zinc-50 disabled:text-zinc-500';

/** 表单和筛选栏共用输入控件，保留原生约束与键盘交互。 */
export function Input({ className, ...props }: ComponentProps<'input'>) {
  return <input className={cn(fieldClass, 'h-9', className)} {...props} />;
}

export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  return (
    <textarea
      className={cn(fieldClass, 'resize-y py-2 leading-6', className)}
      {...props}
    />
  );
}

export function Select({
  className,
  children,
  ...props
}: ComponentProps<'select'>) {
  return (
    <span className={cn('relative block', className)}>
      <select className={cn(fieldClass, 'h-9 appearance-none pr-8')} {...props}>
        {children}
      </select>
      <ChevronDown
        aria-hidden="true"
        size={14}
        className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-zinc-500"
      />
    </span>
  );
}
