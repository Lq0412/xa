import { Link } from '@umijs/max';

/** 未匹配路由提供可用的返回入口。 */
export default function NotFoundPage() {
  return (
    <div className="page-content">
      <h1>页面不存在</h1>
      <Link className="button" to="/chat">
        返回对话
      </Link>
    </div>
  );
}
