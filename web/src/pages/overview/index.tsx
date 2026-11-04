import { PageContainer } from '@ant-design/pro-components';
import { Link } from '@umijs/max';
import { Card, Table, Tag, Typography } from 'antd';
import { adminModules } from '../../../config/modules';

/** 概览展示模块的实际开发状态，不以虚构统计数据填充管理看板。 */
export default function OverviewPage() {
  return (
    <PageContainer
      title="平台概览"
      content="管理 Agent 所需的模型、知识、工具及平台资源。"
    >
      <Card
        title="功能模块"
        extra={<Typography.Text type="secondary">当前建设进度</Typography.Text>}
      >
        <Table
          dataSource={adminModules}
          rowKey="path"
          pagination={false}
          scroll={{ x: 720 }}
          columns={[
            {
              title: '模块',
              dataIndex: 'name',
              width: 180,
              render: (name, item) => <Link to={item.path}>{name}</Link>,
            },
            { title: '职责', dataIndex: 'description' },
            {
              title: '状态',
              width: 130,
              render: (_, item) => (
                <Tag color={item.available ? 'blue' : 'default'}>
                  {item.available ? '基础功能可用' : '待开发'}
                </Tag>
              ),
            },
            {
              title: '入口',
              width: 80,
              render: (_, item) => <Link to={item.path}>进入</Link>,
            },
          ]}
        />
      </Card>
    </PageContainer>
  );
}
