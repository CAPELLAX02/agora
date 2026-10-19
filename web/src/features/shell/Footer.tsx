import { CodeXmlIcon, GraduationCapIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/shared/lib/utils'
import { Avatar } from '@/shared/ui/avatar'
import { Brand } from '@/shared/ui/brand'

/** team, Agora'yı geliştiren ekiptir (bitirme projesi). */
const team = [
  { first: 'Ahmet', last: 'Atar' },
  { first: 'Hasan', last: 'Gülmez' },
  { first: 'Yiğit', last: 'Gülbeyaz' },
] as const

const repository = 'https://github.com/CAPELLAX02/agora'

/**
 * Footer, uygulama kabuğunun altbilgisidir: marka ve kısa tanım, geliştirici ekip, proje
 * bilgisi; en altta telif ve bunun bir bitirme projesi olduğunu söyleyen not.
 */
export function Footer({ className }: { className?: string }) {
  const { t } = useTranslation()
  const year = new Date().getFullYear()
  return (
    <footer className={cn('border-t border-border/60 bg-card/50 print:hidden', className)}>
      <div className="mx-auto grid max-w-6xl gap-8 px-4 py-9 md:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)] md:px-6 lg:px-8">
        <div className="space-y-3">
          <Brand />
          <p className="max-w-sm text-sm leading-relaxed text-muted-foreground">{t('footer.about')}</p>
        </div>
        <section aria-labelledby="footer-team" className="space-y-3">
          <h2
            id="footer-team"
            className="text-xs font-semibold tracking-wide text-muted-foreground uppercase"
          >
            {t('footer.team')}
          </h2>
          <ul className="space-y-2.5">
            {team.map((m) => (
              <li key={m.last} className="flex items-center gap-2.5 text-sm font-medium">
                <Avatar firstName={m.first} lastName={m.last} className="size-7 text-[10px] ring-0" />
                {m.first} {m.last}
              </li>
            ))}
          </ul>
        </section>
        <section aria-labelledby="footer-project" className="space-y-3">
          <h2
            id="footer-project"
            className="text-xs font-semibold tracking-wide text-muted-foreground uppercase"
          >
            {t('footer.project')}
          </h2>
          <ul className="space-y-2.5 text-sm">
            <li className="flex items-center gap-2.5 text-muted-foreground">
              <GraduationCapIcon className="size-4 shrink-0 text-primary" aria-hidden />
              {t('footer.thesis', { year })}
            </li>
            <li>
              <a
                href={repository}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-2.5 rounded-sm font-medium text-foreground focus-ring hover:text-primary"
              >
                <CodeXmlIcon className="size-4 shrink-0 text-primary" aria-hidden />
                {t('footer.source')}
              </a>
            </li>
          </ul>
        </section>
      </div>
      <div className="border-t border-border/60">
        <div className="mx-auto flex max-w-6xl flex-col gap-1.5 px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between md:px-6 lg:px-8">
          <span>
            © {year} Agora · {t('brand.university')}
          </span>
          <span>{t('footer.disclaimer')}</span>
        </div>
      </div>
    </footer>
  )
}

/** AuthFooter, giriş ekranlarının kısa altbilgisidir: telif ve geliştirici ekip. */
export function AuthFooter() {
  const { t } = useTranslation()
  return (
    <footer className="px-4 pb-6 text-center text-xs leading-relaxed text-muted-foreground">
      <p>
        © {new Date().getFullYear()} Agora · {t('brand.university')}
      </p>
      <p>
        {t('footer.team')}: {team.map((m) => `${m.first} ${m.last}`).join(', ')}
      </p>
    </footer>
  )
}
