import { ArrowLeftOutlined } from '@ant-design/icons';
import { history, Link, useLocation, useModel } from '@umijs/max';
import { Alert, Button, Form, Input } from 'antd';
import { useState } from 'react';
import { flushSync } from 'react-dom';
import {
  errorMessage,
  login,
  loginRedirect,
  register,
  TOKEN_KEY,
} from '@/services/api';

/** 用户端邮箱认证直接复用后端账号；登录成功后恢复原访问位置。 */
export default function AuthPage() {
  const location = useLocation();
  const registering = location.pathname === '/register';
  const { setInitialState } = useModel('@@initialState');
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  return (
    <div className="auth-page">
      <Link to="/explore" className="auth-back">
        <ArrowLeftOutlined /> 浏览 Agent
      </Link>
      <div className="auth-card">
        <Link className="brand-mark" to="/explore">
          xa
        </Link>
        <h1>{registering ? '创建你的账号' : '欢迎回来'}</h1>
        <p>
          {registering
            ? '发现 Agent，创建属于自己的助手。'
            : '登录后，继续使用你的 Agent。'}
        </p>
        <Form
          key={location.pathname}
          layout="vertical"
          requiredMark={false}
          onFinish={async (values: {
            email: string;
            password: string;
            display_name?: string;
          }) => {
            setPending(true);
            setError('');
            try {
              if (registering)
                await register(
                  values.email.trim(),
                  values.password,
                  values.display_name?.trim() || '',
                );
              const result = await login(values.email.trim(), values.password);
              localStorage.setItem(TOKEN_KEY, result.token);
              // 跳转前同步身份，避免页面守卫读取上一帧的未登录状态。
              flushSync(() => {
                void setInitialState({ currentUser: result.user });
              });
              history.replace(loginRedirect());
            } catch (cause) {
              setError(errorMessage(cause));
            } finally {
              setPending(false);
            }
          }}
        >
          {error && (
            <Alert type="error" title={error} showIcon className="form-alert" />
          )}
          {registering && (
            <Form.Item
              name="display_name"
              label="昵称"
              rules={[{ max: 80, message: '昵称最多 80 个字符' }]}
            >
              <Input size="large" autoComplete="nickname" />
            </Form.Item>
          )}
          <Form.Item
            name="email"
            label="邮箱"
            rules={[
              { required: true, type: 'email', message: '请输入有效邮箱' },
            ]}
          >
            <Input size="large" autoComplete="email" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[
              { required: true, min: 8, message: '密码至少需要 8 个字符' },
            ]}
          >
            <Input.Password
              size="large"
              autoComplete={registering ? 'new-password' : 'current-password'}
            />
          </Form.Item>
          <Button
            type="primary"
            size="large"
            htmlType="submit"
            block
            loading={pending}
          >
            {registering ? '注册并登录' : '登录'}
          </Button>
        </Form>
        <div className="auth-switch">
          <Link
            onClick={() => setError('')}
            to={`${registering ? '/login' : '/register'}${location.search}`}
          >
            {registering ? '已有账号？登录' : '还没有账号？注册'}
          </Link>
        </div>
      </div>
    </div>
  );
}
