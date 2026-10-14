import {
  ArrowRightIcon,
  CalendarCheck2Icon,
  FileBadgeIcon,
  GraduationCapIcon,
  MegaphoneIcon,
  MonitorPlayIcon,
  ShieldCheckIcon,
  ShieldOffIcon,
  SparklesIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { useRoleName, useScopeLabel } from '@/features/account/scope'
import { usePermissions } from '@/features/auth/usePermissions'
import { navGroups, visible } from '@/features/shell/nav'
import { formatDateTime } from '@/i18n/format'
import { useGetMeQuery, useListMyProgramsQuery, type RoleAssignment } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { AdvisingCard } from './AdvisingCard'
import { DashCard } from './DashCard'
import { GradesCard } from './GradesCard'
import { RegistrationCard } from './RegistrationCard'
import { TeachingCard } from './TeachingCard'
import { WindowsCard } from './WindowsCard'

/**
 * DashboardPage, oturum açan kullanıcının ana sayfasıdır: karşılama bandı (roller, öğrenci
 * için GANO ve yarıyıl), hızlı erişim kutuları, öğrencinin son notları, öğretim elemanının
 * not girişi bekleyen şubeleri, ders kaydı, danışmanlık, işlem pencereleri ve hesap güvenliği.
 */
export function DashboardPage() {
  const { t } = useTranslation()
  const { data: me, error, refetch } = useGetMeQuery()
  const perms = usePermissions()
  const { has, hasAny } = perms

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!me) {
    return (
      <div className="grid gap-5 md:grid-cols-2">
        <Skeleton className="h-44 md:col-span-2" />
        <Skeleton className="h-40" />
        <Skeleton className="h-40" />
      </div>
    )
  }

  // Hesap sayfaları yan menüde ve kullanıcı menüsünde zaten var.
  const shortcuts = navGroups
    .filter((g) => g.label !== 'nav.account')
    .flatMap((g) => g.items)
    .filter((i) => i.to !== '/' && visible(i, perms))
    .slice(0, 8)

  return (
    <div className="space-y-7">
      <Hero firstName={me.first_name} roles={me.roles} student={has('registration:manage_own')} />

      {shortcuts.length > 0 && (
        <section aria-labelledby="shortcuts" className="space-y-3">
          <h2 id="shortcuts" className="text-sm font-bold tracking-wider text-muted-foreground uppercase">
            {t('dashboard.shortcuts.title')}
          </h2>
          <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {shortcuts.map(({ to, label, icon, tone }) => (
              <li key={to}>
                <Link
                  to={to}
                  className="group flex h-full items-center gap-3 rounded-2xl border border-border/70 bg-card p-3.5 text-sm font-semibold shadow-soft transition-all outline-none hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-lift focus-visible:ring-4 focus-visible:ring-ring/30"
                >
                  <IconChip
                    icon={icon}
                    tone={tone}
                    solid
                    className="transition-transform group-hover:scale-110"
                  />
                  <span className="min-w-0 flex-1 leading-tight">{t(label)}</span>
                  <ArrowRightIcon
                    className="size-4 shrink-0 text-muted-foreground opacity-0 transition-all group-hover:translate-x-0.5 group-hover:opacity-100"
                    aria-hidden
                  />
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="grid gap-5 lg:grid-cols-2">
        {has('grade:read_own') && <GradesCard />}
        {has('assessment_plan:manage') && <TeachingCard />}
        {has('registration:manage_own') && <RegistrationCard />}
        {hasAny(['registration:approve', 'registration:approve_add_drop']) && <AdvisingCard />}
        {has('calendar:read') && <WindowsCard />}

        <DashCard
          icon={me.mfa_enabled ? ShieldCheckIcon : ShieldOffIcon}
          tone={me.mfa_enabled ? 'emerald' : 'amber'}
          title={t('dashboard.security.title')}
          description={t('dashboard.security.lastLogin', { time: formatDateTime(me.last_login_at) })}
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Badge variant={me.mfa_enabled ? 'success' : 'warning'}>
              {t(me.mfa_enabled ? 'dashboard.security.mfaOn' : 'dashboard.security.mfaOff')}
            </Badge>
            <Button variant="soft" size="sm" asChild>
              <Link to="/guvenlik">
                {t('dashboard.security.manage')}
                <ArrowRightIcon />
              </Link>
            </Button>
          </div>
        </DashCard>

        <DashCard
          icon={SparklesIcon}
          tone="fuchsia"
          title={
            <span className="flex items-center gap-2">
              {t('dashboard.upcoming.title')}
              <Badge variant="violet">{t('common.comingSoon')}</Badge>
            </span>
          }
          description={t('dashboard.upcoming.description')}
        >
          <ul className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {(
              [
                ['lms', MonitorPlayIcon, 'orange'],
                ['announcements', MegaphoneIcon, 'sky'],
                ['attendance', CalendarCheck2Icon, 'emerald'],
                ['documents', FileBadgeIcon, 'violet'],
              ] as const
            ).map(([key, icon, tone]) => (
              <li
                key={key}
                className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border bg-muted/30 px-2 py-3 text-center text-xs font-semibold text-muted-foreground"
              >
                <IconChip icon={icon} tone={tone} size="sm" />
                {t(`dashboard.upcoming.modules.${key}`)}
              </li>
            ))}
          </ul>
        </DashCard>
      </div>
    </div>
  )
}

/** Hero, ana sayfanın renkli karşılama bandıdır. */
function Hero({
  firstName,
  roles,
  student,
}: {
  firstName: string
  roles: RoleAssignment[]
  student: boolean
}) {
  const { t } = useTranslation()
  const roleName = useRoleName()
  const scopeLabel = useScopeLabel()
  const programs = useListMyProgramsQuery(undefined, { skip: !student })
  const sp = programs.data?.items[0]
  const today = new Intl.DateTimeFormat(document.documentElement.lang, { dateStyle: 'full' }).format(
    new Date(),
  )

  return (
    <section className="relative overflow-hidden rounded-2xl bg-brand-gradient p-6 text-white shadow-lift sm:p-8">
      <div aria-hidden className="pointer-events-none absolute inset-0">
        <div className="absolute -top-20 right-10 size-64 rounded-full bg-amber-300/30 blur-3xl" />
        <div className="absolute -bottom-24 -left-10 size-72 rounded-full bg-sky-300/30 blur-3xl" />
        <GraduationCapIcon className="absolute -right-6 -bottom-8 size-48 rotate-12 text-white/10" />
      </div>
      <div className="relative flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div className="min-w-0 space-y-3">
          <p className="text-sm font-semibold text-white/80">{today}</p>
          <h1 className="text-3xl font-extrabold tracking-tight sm:text-4xl">
            {t('dashboard.greeting', { name: firstName })}
          </h1>
          {sp && (
            <p className="text-sm font-medium text-white/90">
              {sp.program.name_tr} ·{' '}
              {sp.class_level === 0
                ? t('dashboard.hero.prep')
                : t('dashboard.hero.classLevel', { level: sp.class_level })}
            </p>
          )}
          {roles.length > 0 && (
            <ul aria-label={t('dashboard.roles.title')} className="flex flex-wrap gap-2">
              {roles.map((r) => (
                <li
                  key={`${r.role}-${r.scope_id ?? ''}`}
                  className="rounded-full bg-white/15 px-3 py-1 text-xs font-semibold ring-1 ring-white/25 backdrop-blur-md"
                  title={scopeLabel(r)}
                >
                  {roleName(r)}
                  {r.scope_type !== 'NONE' && (
                    <span className="font-medium text-white/75"> · {scopeLabel(r)}</span>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
        {sp && (
          <dl className="grid shrink-0 grid-cols-3 gap-3">
            {(
              [
                ['gpa', sp.gpa === null ? '—' : sp.gpa.toFixed(2)],
                ['semester', String(sp.current_semester)],
                ['ects', String(sp.earned_ects)],
              ] as const
            ).map(([key, value]) => (
              <div
                key={key}
                className="min-w-24 rounded-2xl bg-white/15 px-4 py-3 ring-1 ring-white/25 backdrop-blur-md"
              >
                <dt className="text-xs font-semibold text-white/75">{t(`dashboard.hero.${key}`)}</dt>
                <dd className="tabular text-2xl font-extrabold">{value}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
    </section>
  )
}
