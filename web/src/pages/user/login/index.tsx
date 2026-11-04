import { LockOutlined, MailOutlined } from '@ant-design/icons';
import { LoginForm, ProFormText } from '@ant-design/pro-components';
import { history, Link, useModel } from '@umijs/max';
import { Alert, App } from 'antd';
import { createStyles } from 'antd-style';
import { useState } from 'react';
import { flushSync } from 'react-dom';
import { Footer } from '@/components';
import { ApiError } from '@/requestErrorConfig';
import { login } from '@/services/xa/api';
import { getLoginRedirect, TOKEN_KEY } from '@/services/xa/session';

const useStyles = createStyles(({ token }) => ({
  layout: {
    minHeight: '100vh',
    display: 'flex',
    flexDirection: 'column',
    background: token.colorBgLayout,
  },
  main: {
    flex: 1,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '32px 16px',
  },
  card: { width: 380, maxWidth: '100%' },
}));

/** 使用 Pro LoginForm 登录，提交失败在当前表单显示服务端原因。 */
export default function LoginPage() {
  const { styles } = useStyles();
  const { message } = App.useApp();
  const { setInitialState } = useModel('@@initialState');
  const [error, setError] = useState('');

  return (
    <div className={styles.layout}>
      <main className={styles.main}>
        <LoginForm<{ email: string; password: string; display_name?: string }>
          className={styles.card}
          logo={<img alt="xa" src="/logo.svg" />}
          title="xa 管理控制台"
          subTitle="登录 Agent 管理平台"
          submitter={{ searchConfig: { submitText: '登录' } }}
          onFinish={async (values) => {
            setError('');
            try {
              const result = await login(values.email.trim(), values.password);
              window.localStorage.setItem(TOKEN_KEY, result.token);
              // 在跳转前同步身份，避免布局守卫读到上一帧的未登录状态。
              flushSync(() => {
                void setInitialState({ currentUser: result.user });
              });
              message.success('登录成功');
              history.replace(getLoginRedirect());
              return true;
            } catch (cause) {
              if (!(cause instanceof ApiError)) throw cause;
              setError(cause.message);
              return false;
            }
          }}
          actions={<Link to="/user/register">用户注册与使用入口</Link>}
        >
          {error && (
            <Alert
              title={error}
              type="error"
              showIcon
              style={{ marginBottom: 24 }}
            />
          )}
          <ProFormText
            name="email"
            fieldProps={{
              size: 'large',
              prefix: <MailOutlined />,
              autoComplete: 'email',
              'aria-label': '邮箱',
            }}
            placeholder="邮箱"
            rules={[
              { required: true, type: 'email', message: '请输入有效邮箱' },
            ]}
          />
          <ProFormText.Password
            name="password"
            fieldProps={{
              size: 'large',
              prefix: <LockOutlined />,
              autoComplete: 'current-password',
              'aria-label': '密码',
            }}
            placeholder="密码（至少 8 个字符）"
            rules={[
              { required: true, min: 8, message: '密码至少需要 8 个字符' },
            ]}
          />
        </LoginForm>
      </main>
      <Footer />
    </div>
  );
}
