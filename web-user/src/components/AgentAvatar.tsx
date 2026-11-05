import { cn } from '@/lib/utils';

/** 当前所有 Agent 共用这一张默认头像，不读取后端 avatar 字段。 */
export function AgentAvatar({
  size = 32,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'agent-avatar inline-flex shrink-0 overflow-hidden rounded-full bg-[#fff6d0]',
        className,
      )}
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <img
        src="/default-agent-avatar.png"
        alt=""
        className="h-full w-full object-cover"
      />
    </span>
  );
}
