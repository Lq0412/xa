import { PageContainer } from '@ant-design/pro-components';
import { Link } from '@umijs/max';
import { Card } from 'antd';

/** 概览提供已接入管理功能的入口。 */
export default function OverviewPage() {
  return (
    <PageContainer
      title="平台概览"
      content="维护 Agent 的资料、提示词和可用状态。"
    >
      <Card title="Agent 管理">
        <p>查看和搜索 Agent，创建或编辑资料与提示词，管理启停状态。</p>
        <Link to="/agents">进入 Agent 管理</Link>
      </Card>
    </PageContainer>
  );
}
