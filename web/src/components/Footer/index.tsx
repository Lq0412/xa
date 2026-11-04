import { DefaultFooter } from '@ant-design/pro-components';

/** 使用 ProLayout 默认页脚，产品页面只展示应用归属。 */
export default function Footer() {
  return (
    <DefaultFooter
      copyright={`${new Date().getFullYear()} xa Agent`}
      links={[]}
    />
  );
}
