import { LogoutOutlined } from '@ant-design/icons';
import { history, useModel } from '@umijs/max';
import { App, Dropdown } from 'antd';
import type { ReactNode } from 'react';
import { ApiError } from '@/requestErrorConfig';
import { logout } from '@/services/xa/api';
import { LOGIN_PATH, TOKEN_KEY } from '@/services/xa/session';

/** 退出时先撤销服务端会话，失败则明确提示并保留当前登录状态。 */
export function AvatarDropdown({ children }: { children: ReactNode }) {
  const { setInitialState } = useModel('@@initialState');
  const { message } = App.useApp();

  const signOut = async () => {
    try {
      await logout();
      window.localStorage.removeItem(TOKEN_KEY);
      await setInitialState({});
      history.replace(LOGIN_PATH);
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      message.error(cause.message);
    }
  };

  return (
    <Dropdown
      placement="bottomRight"
      trigger={['click']}
      menu={{
        items: [
          {
            key: 'logout',
            icon: <LogoutOutlined />,
            label: '退出登录',
            onClick: () => void signOut(),
          },
        ],
      }}
    >
      <button type="button" className="account-menu" aria-label="账号菜单">
        {children}
      </button>
    </Dropdown>
  );
}
