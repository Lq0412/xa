import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import type { ButtonHTMLAttributes } from 'react';
import { cn } from '@/lib/utils';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-xl text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#f5c518] disabled:pointer-events-none disabled:opacity-45',
  {
    variants: {
      variant: {
        default: 'bg-[#f5c518] text-[#3a2a00] hover:bg-[#e6b70f]',
        outline:
          'border border-zinc-200 bg-white text-zinc-800 hover:bg-zinc-50',
        ghost: 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900',
        quiet: 'bg-zinc-100 text-zinc-800 hover:bg-zinc-200',
      },
      size: {
        default: 'h-9 px-3.5',
        sm: 'h-8 px-2.5 text-[13px]',
        icon: 'size-9',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
  };

/** 主按钮只用头像同色的暖黄，其余操作保持锌色。 */
export function Button({
  className,
  variant,
  size,
  asChild = false,
  type = 'button',
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot : 'button';
  return (
    <Comp
      className={cn(buttonVariants({ variant, size }), className)}
      {...(!asChild ? { type } : {})}
      {...props}
    />
  );
}

export { buttonVariants };
