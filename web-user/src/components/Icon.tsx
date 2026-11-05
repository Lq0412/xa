import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Bot,
  Check,
  Copy,
  Info,
  LayoutGrid,
  LogOut,
  type LucideProps,
  Menu,
  MessageCircle,
  Pencil,
  Plus,
  Search,
  Settings2,
  Square,
  UserRound,
  UsersRound,
  X,
} from 'lucide-react';

/** 使用同一套 Lucide 线性图标，保持用户端的尺寸与笔画一致。 */
const icons = {
  'arrow-down': ArrowDown,
  'arrow-left': ArrowLeft,
  'arrow-right': ArrowRight,
  'arrow-up': ArrowUp,
  robot: Bot,
  check: Check,
  copy: Copy,
  info: Info,
  grid: LayoutGrid,
  logout: LogOut,
  menu: Menu,
  chat: MessageCircle,
  edit: Pencil,
  plus: Plus,
  search: Search,
  settings: Settings2,
  stop: Square,
  user: UserRound,
  team: UsersRound,
  close: X,
};
export type IconName = keyof typeof icons;

/** 图标含义由外层按钮的可访问名称提供。 */
export function Icon({
  name,
  size = 20,
  ...props
}: LucideProps & { name: IconName }) {
  const Component = icons[name];
  return (
    <Component size={size} strokeWidth={1.7} aria-hidden="true" {...props} />
  );
}
