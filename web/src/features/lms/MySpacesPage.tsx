import { ArrowRightIcon, MonitorPlayIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { TermSelect } from '@/features/calendar/TermSelect'
import { useTermParam } from '@/features/calendar/useTermParam'
import { navTheme } from '@/features/shell/nav'
import { useMySpacesQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Meter } from '@/shared/ui/meter'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { toneFill, toneFor, toneSoft } from '@/shared/ui/tone'

/**
 * MySpacesPage, "Derslerim"dir: dönemde öğrencisi ya da öğretim elemanı olunan derslerin
 * alanları. Öğrencinin kaydı onaylandığı anda dersleri burada.
 */
export function MySpacesPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [termId, setTermId] = useTermParam()
  const { data, error, refetch } = useMySpacesQuery(termId ? { termId } : {}, { skip: !termId })

  return (
    <>
      <PageHeader
        {...navTheme('/derslerim')}
        title={t('lms.mySpaces.title')}
        description={t('lms.mySpaces.description')}
        actions={<TermSelect value={termId} onChange={setTermId} />}
      />
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !data ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <Skeleton className="h-40" />
          <Skeleton className="h-40" />
          <Skeleton className="h-40" />
        </div>
      ) : data.items.length === 0 ? (
        <EmptyState icon={MonitorPlayIcon} tone="indigo">
          {t('lms.mySpaces.empty')}
        </EmptyState>
      ) : (
        <ul className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {data.items.map((s) => {
            const tone = toneFor(s.course.code)
            const primary = s.instructors.find((i) => i.role === 'PRIMARY') ?? s.instructors[0]
            return (
              <li key={s.section_id}>
                <Link
                  to={`/derslerim/${s.section_id}`}
                  className="group relative flex h-full flex-col gap-3 overflow-hidden rounded-2xl border border-border/70 bg-card p-5 pt-6 shadow-soft transition-all outline-none hover:-translate-y-0.5 hover:shadow-lift focus-visible:ring-4 focus-visible:ring-ring/30"
                >
                  <span aria-hidden className={cn('absolute inset-x-0 top-0 h-1.5', toneFill[tone])} />
                  <div className="flex items-start justify-between gap-2">
                    <span
                      className={cn('rounded-md px-1.5 py-0.5 font-mono text-xs font-bold', toneSoft[tone])}
                    >
                      {s.course.code}-{s.section_code}
                    </span>
                    {s.role !== 'STUDENT' && <Badge variant="violet">{t(`lms.roles.${s.role}`)}</Badge>}
                  </div>
                  <h2 className="text-lg leading-snug font-bold">{localized(s.course)}</h2>
                  {primary && (
                    <p className="text-sm text-muted-foreground">
                      {[primary.title, primary.first_name, primary.last_name].filter(Boolean).join(' ')}
                    </p>
                  )}
                  {s.progress && s.progress.total > 0 && (
                    <div className="space-y-1">
                      <Meter
                        value={s.progress.completed}
                        max={s.progress.total}
                        tone="emerald"
                        size="sm"
                        label={t('lms.progress.title')}
                      />
                      <p className="text-xs text-muted-foreground">
                        {t('lms.progress.percent', {
                          percent: Math.round((s.progress.completed / s.progress.total) * 100),
                        })}
                      </p>
                    </div>
                  )}
                  <p className="mt-auto flex items-center justify-between text-xs text-muted-foreground">
                    {t('lms.mySpaces.activities', { count: s.activities })}
                    <ArrowRightIcon
                      className="size-4 transition-transform group-hover:translate-x-0.5"
                      aria-hidden
                    />
                  </p>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}
