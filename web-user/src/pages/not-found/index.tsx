import { Link } from '@umijs/max';
import { Result } from 'antd';

/** 未知地址保留返回 Agent 列表的入口。 */
export default function NotFoundPage() {
  return (
    <Result
      status="404"
      title="页面不存在"
      extra={<Link to="/explore">返回 Agent 列表</Link>}
    />
  );
}
