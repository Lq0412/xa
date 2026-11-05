import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useState,
} from 'react';

const key = 'xa.user.agent-details.open';
const DetailsContext = createContext<{
  open: boolean;
  setOpen: (open: boolean) => void;
} | null>(null);

/** 桌面详情偏好保存在本机；进入窄屏时先收起，仍可由用户主动打开。 */
export function DetailsPreference({ children }: { children: ReactNode }) {
  const [open, updateOpen] = useState(
    () =>
      matchMedia('(min-width: 1200px)').matches &&
      localStorage.getItem(key) !== 'false',
  );
  useEffect(() => {
    const media = matchMedia('(min-width: 1200px)');
    const change = () =>
      updateOpen(media.matches && localStorage.getItem(key) !== 'false');
    media.addEventListener('change', change);
    return () => media.removeEventListener('change', change);
  }, []);
  return (
    <DetailsContext.Provider
      value={{
        open,
        setOpen: (value) => {
          localStorage.setItem(key, String(value));
          updateOpen(value);
        },
      }}
    >
      {children}
    </DetailsContext.Provider>
  );
}

/** 导航设置与对话面板使用同一展开状态。 */
export function useDetailsPreference() {
  const value = useContext(DetailsContext);
  if (!value) throw new Error('详情面板必须位于 DetailsPreference 内');
  return value;
}
