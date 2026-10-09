import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

import { Input } from './input'

/**
 * OtpInput, doğrulayıcı uygulamadaki 6 haneli kod için bir alandır. Tarayıcı ve parola
 * yöneticileri autocomplete="one-time-code" ile kodu otomatik doldurabilir. Rakam
 * dışındaki karakterler (boşluk, tire) silinir.
 */
export function OtpInput({ className, onChange, ...props }: Omit<ComponentProps<'input'>, 'type'>) {
  return (
    <Input
      type="text"
      inputMode="numeric"
      autoComplete="one-time-code"
      pattern="[0-9]*"
      maxLength={6}
      className={cn('h-12 text-center font-mono text-2xl tracking-[0.5em]', className)}
      onChange={(e) => {
        e.target.value = e.target.value.replace(/\D/g, '').slice(0, 6)
        onChange?.(e)
      }}
      {...props}
    />
  )
}
