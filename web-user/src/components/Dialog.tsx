import type { ReactNode } from 'react';
import { useEffect, useId, useRef } from 'react';
import { Icon } from '@/components/Icon';
import {
  DialogContent,
  DialogDescription,
  Dialog as DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';

/** 打开时记住触发控件，关闭后把焦点还回去。渲染阶段记录，避免子效果先把焦点移进弹层。 */
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
  const titleID = useId();
  const trigger = useRef<HTMLElement | null>(null);
  if (
    trigger.current === null &&
    document.activeElement instanceof HTMLElement
  ) {
    trigger.current = document.activeElement;
  }
  useEffect(() => {
    const node = trigger.current;
    return () => {
      if (node?.isConnected) node.focus();
    };
  }, []);
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={
          side
            ? cn(
                'inset-y-0 flex h-dvh w-[min(340px,calc(100vw-48px))] flex-col shadow-xl',
                side === 'left'
                  ? 'left-0 border-r border-zinc-200'
                  : 'right-0 border-l border-zinc-200',
              )
            : 'top-1/2 left-1/2 max-h-[calc(100dvh-48px)] w-[min(480px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 overflow-auto rounded-2xl border border-zinc-200 shadow-xl'
        }
        aria-labelledby={titleID}
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault();
        }}
        onInteractOutside={(event) => {
          if (busy) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (trigger.current?.isConnected) trigger.current.focus();
        }}
      >
        <header className="flex items-center justify-between px-5 py-4">
          <DialogTitle id={titleID} className="text-base font-medium">
            {title}
          </DialogTitle>
          <button
            type="button"
            className="flex size-8 items-center justify-center rounded-full text-zinc-500 hover:bg-zinc-100"
            aria-label={`关闭${title}`}
            disabled={busy}
            onClick={onClose}
          >
            <Icon name="close" size={18} />
          </button>
        </header>
        <DialogDescription className="sr-only">{title}</DialogDescription>
        {children}
      </DialogContent>
    </DialogRoot>
  );
}
