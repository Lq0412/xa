import { Link } from '@umijs/max';

/** 未匹配路由提供可用的返回入口。 */
export default function NotFoundPage() {
  return (
    <div className="px-10 py-16">
      <h1 className="text-2xl font-medium">页面不存在</h1>
      <Link
        className="mt-6 inline-flex h-9 items-center rounded-xl border border-zinc-200 px-3.5 text-sm"
        to="/chat"
      >
        返回对话
      </Link>
    </div>
  );
}
