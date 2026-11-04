import { Link } from '@umijs/max';
import { Button, Result } from 'antd';

/** 未匹配路由提供返回入口，避免展示模板的演示页面。 */
export default function NotFoundPage() {
  return (
    <Result
      status="404"
      title="404"
      subTitle="访问的页面不存在"
      extra={
        <Link to="/agents">
          <Button type="primary">返回 Agent 管理</Button>
        </Link>
      }
    />
  );
}
