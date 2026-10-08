import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/** cn, sınıf adlarını birleştirir. Çakışan Tailwind sınıflarında sonuncusu kazanır. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
