import { Link } from '@umijs/max';
import { Button, Result, Space } from 'antd';

/** 普通账号统一在独立用户端注册，管理员由部署配置初始化。 */
export default function RegisterPage() {
  return (
    <Result
      title="前往用户端创建账号"
      subTitle="注册后可浏览公共 Agent，并创建自己的 Agent。"
      extra={
        <Space orientation="vertical">
          {process.env.XA_USER_WEB_URL && (
            <Button
              type="primary"
              href={`${process.env.XA_USER_WEB_URL}/register`}
            >
              前往注册
            </Button>
          )}
          <Link to="/user/login">返回管理员登录</Link>
        </Space>
      }
    />
  );
}
