import { PageContainer } from '@ant-design/pro-components';
import { history, useLocation } from '@umijs/max';
import { Card, Empty, Table, Tabs, Tag, Typography } from 'antd';
import { adminModules } from '../../../config/modules';

/** 顶层模块占位保留真实的页面分区；尚未接入业务时明确展示开发状态。 */
export default function ModulePage() {
  const location = useLocation();
  const module = adminModules.find((item) => item.path === location.pathname);
  if (!module?.sections.length) throw new Error('管理模块路由未配置');
  const requested = new URLSearchParams(location.search).get('tab');
  const section =
    module.sections.find((item) => item.key === requested) ??
    module.sections[0];
  return (
    <PageContainer
      title={module.name}
      tags={<Tag>待开发</Tag>}
      content={module.description}
    >
      <Card>
        <Tabs
          activeKey={section.key}
          onChange={(key) => history.replace(`${module.path}?tab=${key}`)}
          items={module.sections.map((item) => ({
            key: item.key,
            label: item.title,
          }))}
        />
        <Typography.Paragraph type="secondary">
          {section.description}
        </Typography.Paragraph>
        <Table
          rowKey="id"
          pagination={false}
          dataSource={[]}
          scroll={{ x: 650 }}
          columns={section.columns.map((title) => ({
            title,
            dataIndex: title,
          }))}
          locale={{
            emptyText: (
              <div className="module-placeholder">
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description={
                    <>
                      <Typography.Title level={5}>
                        {section.title}待开发
                      </Typography.Title>
                      <Typography.Text type="secondary">
                        功能接入后，可在这里查看和管理真实数据。
                      </Typography.Text>
                    </>
                  }
                />
              </div>
            ),
          }}
        />
      </Card>
    </PageContainer>
  );
}
