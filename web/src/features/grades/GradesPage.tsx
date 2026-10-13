import type { TFunction } from 'i18next'
import { CalendarCheckIcon, EyeOffIcon, FileTextIcon, UserXIcon, UsersRoundIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import { navTheme } from '@/features/shell/nav'
import { formatDate } from '@/i18n/format'
import { useGetStudentGradesQuery, useListMyProgramsQuery, type StudentGrades } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { LetterGrade } from '@/shared/ui/letter-grade'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { toneFill, toneFor } from '@/shared/ui/tone'

type Course = StudentGrades['courses'][number]
type Component = Course['components'][number]
type Term = StudentGrades['term']

/** termName, "2025-FALL" kodlu dönemi "2025-2026 Güz" biçiminde yazar. */
function termName(t: TFunction, term: Term): string {
  const year = Number(term.code.slice(0, 4))
  const type = t(`calendar.termTypes.${term.term_type}`)
  return Number.isFinite(year) ? `${year}-${year + 1} ${type}` : `${term.code} ${type}`
}

/**
 * GradesPage, öğrencinin bir dönemdeki notlarıdır: her ders için değerlendirme bileşenleri ve
 * ağırlıkları, yayımlanan bileşenlerde notu ve sınıfın ortalaması, devam durumu, harf notu ve
 * sınıf istatistiği. Dönem `?donem=` ile seçilir; verilmezse içinde bulunulan dönem.
 */
export function GradesPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const programs = useListMyProgramsQuery()
  const sp = programs.data?.items[0]
  const termId = params.get('donem') ?? undefined
  const { data, error, refetch, isFetching } = useGetStudentGradesQuery(
    { id: sp?.id ?? '', termId },
    { skip: !sp },
  )

  if (error || programs.error) {
    return <ErrorState error={error ?? programs.error} onRetry={() => void refetch()} />
  }
  if (programs.data && !sp) {
    return <EmptyState {...navTheme('/notlarim')}>{t('grades.noProgram')}</EmptyState>
  }
  return (
    <>
      <PageHeader
        {...navTheme('/notlarim')}
        title={t('grades.title')}
        description={data ? termName(t, data.term) : t('grades.description')}
        actions={
          <Button asChild variant="soft">
            <Link to="/transkript">
              <FileTextIcon />
              {t('grades.transcript')}
            </Link>
          </Button>
        }
      />
      {!data ? (
        <Skeleton className="h-96" />
      ) : data.terms.length === 0 ? (
        <EmptyState {...navTheme('/notlarim')}>{t('grades.none')}</EmptyState>
      ) : (
        <div className="space-y-6">
          <nav aria-label={t('grades.terms')} className="-mx-1 flex gap-1.5 overflow-x-auto px-1 pb-1">
            {[...data.terms].reverse().map((term) => (
              <button
                key={term.id}
                type="button"
                aria-current={term.id === data.term.id ? 'page' : undefined}
                onClick={() => setParams({ donem: term.id })}
                className={cn(
                  'shrink-0 cursor-pointer rounded-full border px-3.5 py-1.5 text-sm font-semibold whitespace-nowrap transition-colors',
                  term.id === data.term.id
                    ? 'border-transparent bg-brand-gradient text-white shadow-soft'
                    : 'border-border bg-card text-muted-foreground hover:text-foreground',
                )}
              >
                {termName(t, term)}
                {term.is_current && (
                  <span className="ml-1.5 text-xs opacity-80">· {t('calendar.current')}</span>
                )}
              </button>
            ))}
          </nav>
          {data.courses.length === 0 ? (
            <EmptyState {...navTheme('/notlarim')}>{t('grades.emptyTerm')}</EmptyState>
          ) : (
            <div className={cn('grid gap-4 lg:grid-cols-2', isFetching && 'opacity-70 transition-opacity')}>
              {data.courses.map((c) => (
                <CourseCard key={c.enrollment_id} course={c} />
              ))}
            </div>
          )}
        </div>
      )}
    </>
  )
}

function CourseCard({ course: c }: { course: Course }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const tone = toneFor(c.course.code)
  const absent = c.attendance_status === 'ATTENDANCE_FAIL'
  return (
    <article
      className="relative overflow-hidden rounded-2xl border bg-card shadow-xs"
      aria-label={`${c.course.code} ${localized(c.course)}`}
    >
      <span aria-hidden className={cn('absolute inset-y-0 left-0 w-1.5', toneFill[tone])} />
      <header className="flex items-start justify-between gap-3 border-b py-3.5 pr-4 pl-5">
        <div className="min-w-0">
          <p className="font-mono text-xs text-muted-foreground">
            {c.course.code}-{c.section_code} · {c.ects} AKTS
          </p>
          <h2 className="leading-snug font-bold">{localized(c.course)}</h2>
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {absent && (
              <Badge variant="destructive" className="gap-1">
                <UserXIcon className="size-3.5" aria-hidden />
                {t('grades.absent')}
              </Badge>
            )}
            {c.grade_published_at && (
              <Badge variant="muted" className="gap-1">
                <CalendarCheckIcon className="size-3.5" aria-hidden />
                {t('grades.publishedOn', { date: formatDate(c.grade_published_at) })}
              </Badge>
            )}
          </div>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1">
          {c.letter_grade ? (
            <LetterGrade letter={c.letter_grade} className="min-w-12 py-1 text-base" />
          ) : (
            <Badge variant="info">{t('transcript.ongoing')}</Badge>
          )}
          {c.weighted_score !== null && (
            <span className="text-xs font-semibold text-muted-foreground tabular-nums">
              {c.weighted_score.toFixed(2)}
            </span>
          )}
        </div>
      </header>
      {c.class_stats && (
        <p className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b bg-muted/30 py-2 pr-4 pl-5 text-sm">
          <span className="inline-flex items-center gap-1.5 text-muted-foreground">
            <UsersRoundIcon className="size-4" aria-hidden />
            {t('grades.classMean')}
          </span>
          <strong className="tabular-nums">{c.class_stats.mean?.toFixed(2) ?? '—'}</strong>
          <span className="text-xs text-muted-foreground">
            {t('grades.classSize', { count: c.class_stats.student_count })}
          </span>
        </p>
      )}
      {c.components.length > 0 ? (
        <ul className="divide-y" aria-label={t('grades.components')}>
          {c.components.map((comp) => (
            <ComponentRow key={comp.id} component={comp} />
          ))}
        </ul>
      ) : (
        // Notu verilmiş ama bileşeni olmayan ders eski sistemden aktarılmıştır: sadece harfi görünür.
        !c.letter_grade && (
          <p className="py-4 pr-4 pl-5 text-sm text-muted-foreground">{t('grades.noPlan')}</p>
        )
      )}
    </article>
  )
}

function ComponentRow({ component: comp }: { component: Component }) {
  const { t, i18n } = useTranslation()
  const label = i18n.language.startsWith('en') ? comp.label_en : comp.label_tr
  const published = comp.published_at !== null
  return (
    <li className="space-y-1.5 py-3 pr-4 pl-5">
      <div className="flex items-center justify-between gap-3 text-sm">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate font-medium">{label}</span>
          <Badge variant="outline" className="shrink-0 tabular-nums">
            %{comp.weight}
          </Badge>
        </span>
        {!published ? (
          <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
            <EyeOffIcon className="size-3.5" aria-hidden />
            {t('grades.notPublished')}
          </span>
        ) : comp.absent ? (
          <Badge variant="destructive">{t('grades.didNotSit')}</Badge>
        ) : (
          <span className="shrink-0 text-base font-extrabold tabular-nums">{comp.score ?? '—'}</span>
        )}
      </div>
      {published && (comp.score !== null || comp.class_average !== null) && (
        <div className="flex items-center gap-3">
          <ScoreBar score={comp.score} average={comp.class_average} />
          {comp.class_average !== null && (
            <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
              {t('grades.average', { value: comp.class_average.toFixed(2) })}
            </span>
          )}
        </div>
      )}
    </li>
  )
}

/** ScoreBar, puanı 100 üzerinden bir çubukla, sınıf ortalamasını çubuğun üstünde bir çizgiyle gösterir. */
function ScoreBar({ score, average }: { score: number | null; average: number | null }) {
  const { t } = useTranslation()
  const above = score !== null && average !== null && score >= average
  return (
    <div
      role="meter"
      aria-label={t('grades.scoreBar')}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={score ?? 0}
      className="relative h-2 flex-1 overflow-visible rounded-full bg-muted"
    >
      {score !== null && (
        <div
          className={cn('h-full rounded-full', above ? 'bg-emerald-500' : 'bg-amber-500')}
          style={{ width: `${Math.min(Math.max(score, 0), 100)}%` }}
        />
      )}
      {average !== null && (
        <span
          aria-hidden
          className="absolute -top-1 h-4 w-0.5 rounded-full bg-foreground/70"
          style={{ left: `calc(${Math.min(Math.max(average, 0), 100)}% - 1px)` }}
        />
      )}
    </div>
  )
}
