import {
  BookOutlined,
  HistoryOutlined,
  TrophyOutlined,
} from '@ant-design/icons';
import { useLocation, useModel } from '@umijs/max';
import { Descriptions, Empty, Tag } from 'antd';

/** 用户能力目录仅描述已确定范围；数据接入前不展示虚构记录或分数。 */
const modules = {
  '/history': {
    title: '会话历史',
    description: '回到曾经的对话，继续交流或回顾内容。',
    icon: <HistoryOutlined />,
    features: ['历史会话列表', '搜索与继续对话', '删除自己的会话'],
    empty: '会话历史待开发',
  },
  '/memory': {
    title: '个人记忆',
    description: '查看和管理各个 Agent 对你的长期记忆。',
    icon: <BookOutlined />,
    features: ['按 Agent 查看记忆', '更正和删除记忆', '控制记忆使用'],
    empty: '个人记忆待开发',
  },
  '/feedback': {
    title: '学习反馈',
    description: '回顾练习中的评价、纠错与改进建议。',
    icon: <TrophyOutlined />,
    features: ['练习反馈记录', '评分依据与纠错', '关联对话和录音'],
    empty: '学习反馈待开发',
  },
};

/** 账号展示真实身份，其他页面呈现用途、功能边界及明确开发状态。 */
export default function ModulePage() {
  const { pathname } = useLocation();
  const { initialState } = useModel('@@initialState');
  if (pathname === '/settings')
    return (
      <div className="page-content">
        <header className="page-heading">
          <div>
            <span className="eyebrow">ACCOUNT</span>
            <h1>账号设置</h1>
            <p>管理个人资料与账号安全。</p>
          </div>
        </header>
        <div className="settings-section">
          <h2>个人资料</h2>
          <Descriptions
            column={1}
            items={[
              {
                key: 'name',
                label: '昵称',
                children: initialState?.currentUser?.display_name || '未设置',
              },
              {
                key: 'email',
                label: '邮箱',
                children: initialState?.currentUser?.email,
              },
              {
                key: 'role',
                label: '账号角色',
                children:
                  initialState?.currentUser?.role === 'admin'
                    ? '管理员'
                    : '普通用户',
              },
            ]}
          />
        </div>
        <div className="settings-section">
          <div className="section-heading">
            <h2>账号安全</h2>
            <Tag>待开发</Tag>
          </div>
          <p className="muted">资料修改、邮箱验证与密码找回将在这里提供。</p>
        </div>
      </div>
    );
  const module = modules[pathname as keyof typeof modules];
  if (!module) throw new Error('用户模块路由未配置');
  return (
    <div className="page-content">
      <header className="page-heading">
        <div>
          <span className="eyebrow">PERSONAL</span>
          <h1>
            {module.title} <Tag>待开发</Tag>
          </h1>
          <p>{module.description}</p>
        </div>
      </header>
      <div className="feature-outline">
        {module.features.map((feature) => (
          <div key={feature}>
            <span>{feature}</span>
            <Tag>待开发</Tag>
          </div>
        ))}
      </div>
      <div className="personal-empty">
        <div className="empty-feature-icon">{module.icon}</div>
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={
            <>
              <h2>{module.empty}</h2>
              <p>功能接入后，你的记录会出现在这里。</p>
            </>
          }
        />
      </div>
    </div>
  );
}
