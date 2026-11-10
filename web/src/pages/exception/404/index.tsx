import { Link } from 'umi';
import { ResultPage } from '@/components/Page';
import { Button } from '@/components/ui/button';

/** 未匹配路由返回统一的管理入口。 */
export default function NotFoundPage() {
  return (
    <ResultPage title="404" description="访问的页面不存在">
      <Button asChild>
        <Link to="/agents">返回 Agent 管理</Link>
      </Button>
    </ResultPage>
  );
}
