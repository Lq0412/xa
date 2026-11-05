import { cn } from '@/lib/utils';

/** 历史加载时的占位，不代表真实消息。 */
export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        'animate-pulse rounded-2xl bg-zinc-200/80 motion-reduce:animate-none',
        className,
      )}
    />
  );
}
