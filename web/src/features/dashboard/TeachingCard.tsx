import { ArrowRightIcon, ClipboardPenLineIcon, ShieldAlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { pendingComponents } from '@/features/grading/pending'
import { formatDateTime } from '@/i18n/format'
import { useGetCurrentTermQuery, useGetGradebookQuery, useMySectionsQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Skeleton } from '@/shared/ui/skeleton'
import { toneFor, toneSoft } from '@/shared/ui/tone'

import { DashCard } from './DashCard'

/**
 * TeachingCard, öğretim elemanının içinde bulunulan dönemde not girişi bekleyen şubeleridir:
 * penceresi açık bileşenler ve eksik not sayısı; şubeden not tablosuna geçilir.
 */
export function TeachingCard() {
  const { t } = useTranslation()
  const term = useGetCurrentTermQuery()
  const teaching = useMySectionsQuery({ termId: term.data?.id ?? '' }, { skip: !term.data })
  if (term.isError || teaching.isError) {
    return null
  }
  const sections = teaching.data?.items ?? []
  return (
    <DashCard
      icon={ClipboardPenLineIcon}
      tone="amber"
      title={t('dashboard.teaching.title')}
      description={t('dashboard.teaching.description')}
    >
      {!teaching.data ? (
        <Skeleton className="h-28" />
      ) : sections.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('dashboard.teaching.none')}</p>
      ) : (
        <ul className="grid grid-cols-[minmax(0,1fr)] gap-2">
          {sections.map((e) => (
            <SectionRow
              key={e.section_id}
              id={e.section_id}
              code={`${e.course.code}-${e.section_code}`}
              course={e.course}
            />
          ))}
        </ul>
      )}
    </DashCard>
  )
}

function SectionRow({
  id,
  code,
  course,
}: {
  id: string
  code: string
  course: { code: string; name_tr: string; name_en: string }
}) {
  const { t, i18n } = useTranslation()
  const localized = useLocalized()
  const { data } = useGetGradebookQuery({ id })
  const en = i18n.language.startsWith('en')
  const pending = data ? pendingComponents(data) : []
  const tone = toneFor(course.code)
  return (
    <li className="min-w-0">
      <Link
        to={`/subeler/${id}/notlar`}
        className="group flex items-center gap-3 rounded-xl border border-border/70 px-3 py-2.5 text-sm transition-all hover:border-primary/25 hover:bg-primary/[0.03]"
      >
        <span className={cn('rounded-md px-1.5 py-0.5 font-mono text-xs font-bold', toneSoft[tone])}>
          {code}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate font-semibold">{localized(course)}</span>
          {!data ? (
            <Skeleton className="mt-1 h-3 w-40" />
          ) : !data.section.plan_locked && data.section.instructor_role !== 'ASSISTANT' ? (
            <span className="text-xs text-amber-700 dark:text-amber-300">
              {t('dashboard.teaching.planNotLocked')}
            </span>
          ) : pending.length === 0 ? (
            <span className="text-xs text-muted-foreground">{t('dashboard.teaching.upToDate')}</span>
          ) : (
            <span className="block text-xs text-amber-700 dark:text-amber-300">
              {pending
                .map((p) =>
                  t('dashboard.teaching.pending', {
                    component: en ? p.component.label_en : p.component.label_tr,
                    count: p.missing,
                    time: formatDateTime(p.component.entry_window.ends_at),
                  }),
                )
                .join(' · ')}
            </span>
          )}
        </span>
        {data?.mfa_required && pending.length > 0 && (
          <Badge variant="warning" className="gap-1">
            <ShieldAlertIcon className="size-3" />
            {t('dashboard.teaching.mfa')}
          </Badge>
        )}
        {pending.length > 0 && (
          <span className="tabular inline-flex min-w-7 justify-center rounded-md bg-amber-500/15 px-2 py-0.5 text-sm font-bold text-amber-800 dark:text-amber-300">
            {pending.reduce((n, p) => n + p.missing, 0)}
          </span>
        )}
        <ArrowRightIcon
          className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5"
          aria-hidden
        />
      </Link>
    </li>
  )
}
