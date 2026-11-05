import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

/** 合并条件类名，后写的 Tailwind 工具类覆盖冲突项。 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
