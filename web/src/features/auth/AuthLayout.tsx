import {
  BookOpenIcon,
  CalendarClockIcon,
  CircleCheckIcon,
  GlobeIcon,
  ShieldCheckIcon,
  SparklesIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { AuthFooter } from '@/features/shell/Footer'
import { PreferenceButtons } from '@/features/shell/Preferences'
import { Brand } from '@/shared/ui/brand'
import { IconChip } from '@/shared/ui/icon-chip'

/**
 * AuthLayout, giriş ve parola ekranlarının iki sütunlu düzenidir: solda marka geçişi,
 * yüzen renkli şekiller ve öğrencinin Agora'da göreceklerinden örnek kartlar; sağda form.
 */
export function AuthLayout({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const features = [
    { icon: BookOpenIcon, text: t('app.features.oneplace') },
    { icon: ShieldCheckIcon, text: t('app.features.secure') },
    { icon: GlobeIcon, text: t('app.features.anywhere') },
  ]
  return (
    <div className="grid min-h-svh lg:grid-cols-[minmax(0,6fr)_minmax(0,5fr)]">
      <aside className="relative hidden overflow-hidden bg-brand-gradient text-white lg:flex lg:flex-col lg:justify-between lg:p-12">
        {/* Yüzen şekiller ve nokta deseni: sadece süs. */}
        <div aria-hidden className="pointer-events-none absolute inset-0">
          <div className="absolute -top-24 -left-24 size-96 rounded-full bg-sky-300/30 blur-3xl" />
          <div className="absolute top-1/3 -right-20 size-80 rounded-full bg-amber-300/30 blur-3xl" />
          <div className="absolute -bottom-32 left-1/4 size-[28rem] rounded-full bg-pink-400/30 blur-3xl" />
          <div
            className="absolute inset-0 opacity-[0.12]"
            style={{
              backgroundImage: 'radial-gradient(white 1px, transparent 1px)',
              backgroundSize: '22px 22px',
            }}
          />
        </div>

        <Brand inverse className="relative [&_img]:size-12" />

        <div className="relative grid max-w-xl gap-10">
          <p className="text-4xl leading-[1.15] font-bold tracking-tight text-balance">{t('app.tagline')}</p>

          {/* Öğrencinin göreceklerinden örnekler. */}
          <div aria-hidden className="relative h-44">
            <div className="absolute top-0 left-0 flex w-72 -rotate-2 items-center gap-3 rounded-xl bg-white p-3.5 text-slate-800 shadow-2xl shadow-indigo-950/30">
              <IconChip icon={CircleCheckIcon} tone="emerald" solid />
              <div className="min-w-0 text-sm">
                <p className="font-bold">{t('app.showcase.approved')}</p>
                <p className="text-xs text-slate-500">{t('app.showcase.approvedHint')}</p>
              </div>
            </div>
            <div className="absolute top-20 left-40 flex w-72 rotate-2 items-center gap-3 rounded-xl bg-white p-3.5 text-slate-800 shadow-2xl shadow-indigo-950/30">
              <IconChip icon={CalendarClockIcon} tone="sky" solid />
              <div className="min-w-0 text-sm">
                <p className="font-bold">{t('app.showcase.today')}</p>
                <p className="text-xs text-slate-500">{t('app.showcase.todayHint')}</p>
              </div>
            </div>
            <div className="absolute top-2 left-[19rem] flex items-center gap-2 rounded-md bg-white/15 px-3 py-1.5 text-sm font-bold ring-1 ring-white/30 backdrop-blur-md">
              <SparklesIcon className="size-4 text-amber-200" />
              {t('app.showcase.gpa')} 3,42
            </div>
          </div>

          <ul className="grid gap-3">
            {features.map(({ icon: Icon, text }) => (
              <li
                key={text}
                className="flex items-center gap-3 rounded-xl bg-white/10 px-3.5 py-2.5 text-sm font-medium ring-1 ring-white/20 backdrop-blur-md"
              >
                <span className="flex size-8 items-center justify-center rounded-full bg-white/20">
                  <Icon className="size-4" aria-hidden />
                </span>
                {text}
              </li>
            ))}
          </ul>
        </div>
        <p className="relative text-xs text-white/70">{t('footer.disclaimer')}</p>
      </aside>

      <div className="flex flex-col app-glow">
        <header className="flex items-center justify-between p-4 lg:justify-end">
          <Brand className="lg:hidden" />
          <PreferenceButtons />
        </header>
        <main id="main" className="flex flex-1 items-center justify-center px-4 pb-16">
          <div className="w-full max-w-sm">{children}</div>
        </main>
        <AuthFooter />
      </div>
    </div>
  )
}
