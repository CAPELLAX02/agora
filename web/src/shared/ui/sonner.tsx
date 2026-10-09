import { Toaster as Sonner, type ToasterProps } from 'sonner'

import { useTheme } from '@/app/theme-context'

/** Toaster, kısa bildirimleri (toast) gösterir. Temaya uyar. */
export function Toaster(props: ToasterProps) {
  const { resolved } = useTheme()
  return (
    <Sonner
      theme={resolved}
      className="toaster group"
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
          '--border-radius': 'var(--radius)',
        } as React.CSSProperties
      }
      {...props}
    />
  )
}
