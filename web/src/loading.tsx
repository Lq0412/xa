import { Skeleton } from '@/components/ui/skeleton';

/** 路由模块和登录态加载时使用同一组占位。 */
export default function Loading() {
  return (
    <div aria-busy="true" className="space-y-6 p-8">
      <Skeleton className="h-7 w-40" />
      <Skeleton className="h-36 w-full" />
    </div>
  );
}
