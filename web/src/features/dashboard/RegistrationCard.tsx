import { ArrowRightIcon, ClipboardCheckIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { EctsMeter } from '@/features/registration/Basket'
import { RegistrationStatusBadge } from '@/features/registration/badges'
import { useGetRegistrationQuery, useListMyProgramsQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Skeleton } from '@/shared/ui/skeleton'
import { toneFill, toneFor } from '@/shared/ui/tone'

import { DashCard } from './DashCard'

/**
 * RegistrationCard, öğrencinin içinde bulunulan dönemdeki ders seçme durumudur: sepet ve AKTS,
 * danışman kararı, ders seçme penceresi açıksa ders seçimine kısayol.
 */
export function RegistrationCard() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const programs = useListMyProgramsQuery()
  const program = programs.data?.items[0]
  const { data: r, isError } = useGetRegistrationQuery({ id: program?.id ?? '' }, { skip: !program })
  if (programs.data && !program) {
    return null
  }
  if (isError) {
    return null // aktif dönem yoksa kart gösterilmez
  }
  return (
    <DashCard
      icon={ClipboardCheckIcon}
      tone="violet"
      title={t('dashboard.registration.title')}
      description={r ? t('registration.termTitle', { term: r.term.code }) : undefined}
      action={r && <RegistrationStatusBadge status={r.status} />}
    >
      {!r ? (
        <Skeleton className="h-24" />
      ) : (
        <>
          <div className="flex items-baseline gap-2">
            <span className="tabular text-3xl font-bold">{r.items.length}</span>
            <span className="text-sm text-muted-foreground">
              {t('dashboard.registration.coursesWord', { count: r.items.length })}
            </span>
          </div>
          <EctsMeter total={r.total_ects} limit={r.ects_limit} />
          {r.items.length > 0 && (
            <ul className="space-y-1.5" aria-label={t('registration.basket.title')}>
              {r.items.slice(0, 4).map((it) => (
                <li key={it.id} className="flex items-center gap-2.5 text-sm">
                  <span
                    aria-hidden
                    className={cn(
                      'size-2.5 shrink-0 rounded-full',
                      toneFill[toneFor(it.section.course.code)],
                    )}
                  />
                  <span className="font-mono text-xs font-semibold text-muted-foreground">
                    {it.section.label}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{localized(it.section.course)}</span>
                  <span className="tabular text-xs text-muted-foreground">
                    {t('registration.ects', { ects: it.ects })}
                  </span>
                </li>
              ))}
              {r.items.length > 4 && (
                <li className="pl-5 text-xs font-medium text-muted-foreground">
                  {t('dashboard.registration.more', { count: r.items.length - 4 })}
                </li>
              )}
            </ul>
          )}
          {r.status === 'REJECTED' && r.decision_note && (
            <p className="rounded-xl bg-rose-500/8 px-3 py-2 text-sm text-rose-700 dark:text-rose-300">
              {r.decision_note}
            </p>
          )}
          <Button
            variant={r.actions.can_edit ? 'default' : 'soft'}
            size="sm"
            className="mt-auto w-fit"
            asChild
          >
            <Link to="/ders-secimi">
              {r.actions.can_edit ? t('dashboard.registration.go') : t('dashboard.registration.view')}
              <ArrowRightIcon />
            </Link>
          </Button>
        </>
      )}
    </DashCard>
  )
}
