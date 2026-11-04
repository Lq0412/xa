import type { ProLayoutProps } from '@ant-design/pro-components';

// 沿用 ProLayout 的标准侧边导航和 Ant Design 默认主题。
const settings: ProLayoutProps = {
  navTheme: 'light',
  colorPrimary: '#1677ff',
  layout: 'side',
  contentWidth: 'Fluid',
  fixedHeader: true,
  fixSiderbar: true,
  title: 'xa Agent',
  logo: '/logo.svg',
};

export default settings;
