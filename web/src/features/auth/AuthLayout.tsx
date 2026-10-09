import { BookOpenIcon, GlobeIcon, ShieldCheckIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PreferenceButtons } from '@/features/shell/Preferences'
import { Brand } from '@/shared/ui/brand'

/** AuthLayout, giriş ve parola ekranlarının iki sütunlu düzenidir. */
export function AuthLayout({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const features = [
    { icon: BookOpenIcon, text: t('app.features.oneplace') },
    { icon: ShieldCheckIcon, text: t('app.features.secure') },
    { icon: GlobeIcon, text: t('app.features.anywhere') },
  ]
  return (
    <div className="grid min-h-svh lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
      <aside className="relative hidden overflow-hidden bg-brand text-brand-foreground lg:flex lg:flex-col lg:justify-between lg:p-12">
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0 opacity-30"
          style={{
            backgroundImage:
              'radial-gradient(circle at 20% 15%, var(--highlight) 0, transparent 35%), radial-gradient(circle at 85% 80%, oklch(1 0 0 / 0.25) 0, transparent 40%)',
          }}
        />
        <Brand className="relative [&_path]:fill-brand-foreground [&_rect]:fill-brand-foreground/15" />
        <div className="relative max-w-md space-y-8">
          <p className="text-3xl leading-tight font-semibold text-balance">{t('app.tagline')}</p>
          <ul className="space-y-4">
            {features.map(({ icon: Icon, text }) => (
              <li key={text} className="flex items-center gap-3 text-sm opacity-90">
                <span className="flex size-9 items-center justify-center rounded-lg bg-brand-foreground/10">
                  <Icon className="size-4" aria-hidden />
                </span>
                {text}
              </li>
            ))}
          </ul>
        </div>
        <p className="relative text-xs opacity-70">© {new Date().getFullYear()} Agora</p>
      </aside>
      <div className="flex flex-col">
        <header className="flex items-center justify-between p-4 lg:justify-end">
          <Brand className="lg:hidden" />
          <PreferenceButtons />
        </header>
        <main id="main" className="flex flex-1 items-center justify-center px-4 pb-16">
          <div className="w-full max-w-sm">{children}</div>
        </main>
      </div>
    </div>
  )
}
