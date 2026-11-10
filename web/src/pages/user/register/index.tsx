import { Link } from 'umi';
import { ResultPage } from '@/components/Page';
import { Button } from '@/components/ui/button';

/** 普通账号在独立用户端注册，管理员由部署配置初始化。 */
export default function RegisterPage() {
  const userWeb = process.env.XA_USER_WEB_URL;
  return (
    <ResultPage
      title="前往用户端创建账号"
      description="注册后可浏览公共 Agent，并创建自己的 Agent。"
    >
      <div className="flex flex-wrap justify-center gap-3">
        {userWeb && (
          <Button asChild>
            <a href={`${userWeb}/register`}>前往注册</a>
          </Button>
        )}
        <Button variant="ghost" asChild>
          <Link to="/user/login">返回管理员登录</Link>
        </Button>
      </div>
    </ResultPage>
  );
}
