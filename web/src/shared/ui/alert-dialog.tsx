import type { LucideIcon } from 'lucide-react'
import { AlertDialog as AlertDialogPrimitive } from 'radix-ui'
import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

import { buttonVariants } from './button'
import { IconChip } from './icon-chip'
import type { Tone } from './tone'

export const AlertDialog = AlertDialogPrimitive.Root
export const AlertDialogTrigger = AlertDialogPrimitive.Trigger

export function AlertDialogContent({
  className,
  ...props
}: ComponentProps<typeof AlertDialogPrimitive.Content>) {
  return (
    <AlertDialogPrimitive.Portal>
      <AlertDialogPrimitive.Overlay className="fixed inset-0 z-50 bg-slate-950/45 backdrop-blur-[3px] data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0" />
      <AlertDialogPrimitive.Content
        className={cn(
          'fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-5 rounded-xl border border-border/60 bg-card p-6 shadow-lift duration-200 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 sm:max-w-lg',
          className,
        )}
        {...props}
      />
    </AlertDialogPrimitive.Portal>
  )
}

/** AlertDialogHeader, başlık ve açıklamadır; icon verilirse solda renkli bir ikon kutusu çıkar. */
export function AlertDialogHeader({
  className,
  icon,
  tone = 'indigo',
  children,
  ...props
}: ComponentProps<'div'> & { icon?: LucideIcon; tone?: Tone }) {
  if (!icon) {
    return (
      <div className={cn('flex flex-col gap-1.5 text-left', className)} {...props}>
        {children}
      </div>
    )
  }
  return (
    <div className={cn('flex items-start gap-3.5 text-left', className)} {...props}>
      <IconChip icon={icon} tone={tone} size="lg" />
      <div className="flex min-w-0 flex-col gap-1.5 pt-0.5">{children}</div>
    </div>
  )
}

export function AlertDialogFooter({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div className={cn('flex flex-col-reverse gap-2 sm:flex-row sm:justify-end', className)} {...props} />
  )
}

export function AlertDialogTitle({ className, ...props }: ComponentProps<typeof AlertDialogPrimitive.Title>) {
  return (
    <AlertDialogPrimitive.Title className={cn('text-lg leading-tight font-bold', className)} {...props} />
  )
}

export function AlertDialogDescription({
  className,
  ...props
}: ComponentProps<typeof AlertDialogPrimitive.Description>) {
  return (
    <AlertDialogPrimitive.Description className={cn('text-sm text-muted-foreground', className)} {...props} />
  )
}

export function AlertDialogAction({
  className,
  variant = 'default',
  ...props
}: ComponentProps<typeof AlertDialogPrimitive.Action> & { variant?: 'default' | 'destructive' }) {
  return <AlertDialogPrimitive.Action className={cn(buttonVariants({ variant }), className)} {...props} />
}

export function AlertDialogCancel({
  className,
  ...props
}: ComponentProps<typeof AlertDialogPrimitive.Cancel>) {
  return (
    <AlertDialogPrimitive.Cancel
      className={cn(buttonVariants({ variant: 'outline' }), className)}
      {...props}
    />
  )
}
