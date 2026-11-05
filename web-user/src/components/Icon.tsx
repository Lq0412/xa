import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Bot,
  Check,
  Copy,
  History,
  Info,
  LayoutGrid,
  Lock,
  LogOut,
  type LucideProps,
  Menu,
  MessageCircle,
  PanelRight,
  Pencil,
  Pin,
  Plus,
  Search,
  Settings2,
  Square,
  Trash2,
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
  history: History,
  info: Info,
  grid: LayoutGrid,
  lock: Lock,
  logout: LogOut,
  menu: Menu,
  panel: PanelRight,
  chat: MessageCircle,
  edit: Pencil,
  pin: Pin,
  plus: Plus,
  search: Search,
  settings: Settings2,
  stop: Square,
  trash: Trash2,
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
