import { type ReactNode, useEffect, useId, useRef } from 'react';
import { Icon } from './Icon';

/** 原生 dialog 提供焦点约束、Escape 关闭和背景隔离，关闭后恢复触发控件的焦点。 */
export function Dialog({
  title,
  children,
  onClose,
  busy = false,
  side,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  busy?: boolean;
  side?: 'left' | 'right';
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleID = useId();
  useEffect(() => {
    const node = ref.current;
    const trigger = document.activeElement;
    node?.showModal();
    return () => {
      node?.close();
      // React 卸载后 dialog 已离开文档，浏览器不会自动恢复原焦点。
      if (trigger instanceof HTMLElement && trigger.isConnected)
        trigger.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={side ? `sheet sheet-${side}` : 'dialog'}
      aria-labelledby={titleID}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <header className="dialog-header">
        <h2 id={titleID}>{title}</h2>
        <button
          type="button"
          className="icon-button"
          aria-label={`关闭${title}`}
          disabled={busy}
          onClick={onClose}
        >
          <Icon name="close" />
        </button>
      </header>
      {children}
    </dialog>
  );
}
