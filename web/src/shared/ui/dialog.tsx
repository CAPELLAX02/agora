import { XIcon, type LucideIcon } from 'lucide-react'
import { Dialog as DialogPrimitive } from 'radix-ui'
import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

import { IconChip } from './icon-chip'
import type { Tone } from './tone'

export const Dialog = DialogPrimitive.Root
export const DialogTrigger = DialogPrimitive.Trigger
export const DialogClose = DialogPrimitive.Close

export function DialogOverlay({ className, ...props }: ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        'fixed inset-0 z-50 bg-slate-950/45 backdrop-blur-[3px] data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0',
        className,
      )}
      {...props}
    />
  )
}

export function DialogContent({
  className,
  children,
  closeLabel,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content> & {
  /** closeLabel, sağ üstteki kapatma düğmesinin erişilebilir adıdır. Verilmezse düğme gösterilmez. */
  closeLabel?: string
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        className={cn(
          'fixed top-[50%] left-[50%] z-50 grid max-h-[calc(100svh-2rem)] w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-5 overflow-y-auto rounded-2xl border border-border/60 bg-card p-6 shadow-lift duration-200 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 sm:max-w-lg',
          className,
        )}
        {...props}
      >
        {children}
        {closeLabel && (
          <DialogPrimitive.Close className="absolute top-4 right-4 cursor-pointer rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-4 focus-visible:ring-ring/30 focus-visible:outline-none disabled:pointer-events-none [&_svg]:size-4">
            <XIcon />
            <span className="sr-only">{closeLabel}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}

/**
 * DialogHeader, başlık ve açıklamayı toplar. icon verilirse başlığın solunda renkli bir
 * ikon kutusu gösterilir.
 */
export function DialogHeader({
  className,
  icon,
  tone = 'indigo',
  children,
  ...props
}: ComponentProps<'div'> & { icon?: LucideIcon; tone?: Tone }) {
  if (!icon) {
    return (
      <div className={cn('flex flex-col gap-1.5 pr-6 text-left', className)} {...props}>
        {children}
      </div>
    )
  }
  return (
    <div className={cn('flex items-start gap-3.5 pr-6 text-left', className)} {...props}>
      <IconChip icon={icon} tone={tone} size="lg" />
      <div className="flex min-w-0 flex-col gap-1.5 pt-0.5">{children}</div>
    </div>
  )
}

export function DialogFooter({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div className={cn('flex flex-col-reverse gap-2 sm:flex-row sm:justify-end', className)} {...props} />
  )
}

export function DialogTitle({ className, ...props }: ComponentProps<typeof DialogPrimitive.Title>) {
  return <DialogPrimitive.Title className={cn('text-lg leading-tight font-bold', className)} {...props} />
}

export function DialogDescription({
  className,
  ...props
}: ComponentProps<typeof DialogPrimitive.Description>) {
  return <DialogPrimitive.Description className={cn('text-sm text-muted-foreground', className)} {...props} />
}
