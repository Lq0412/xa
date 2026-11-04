import { Skeleton } from 'antd';

// 路由模块加载期间使用 Ant Design 骨架屏保留内容位置。
const Loading: React.FC = () => (
  <Skeleton style={{ padding: '24px 40px', height: '60vh' }} active />
);

export default Loading;
