import { ArrowRightIcon, BookOpenCheckIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { formatRelative } from '@/i18n/format'
import { useGetStudentGradesQuery, useListMyProgramsQuery, type StudentGrades } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { LetterGrade } from '@/shared/ui/letter-grade'
import { Skeleton } from '@/shared/ui/skeleton'
import { toneFor, toneSoft } from '@/shared/ui/tone'
import { cn } from '@/shared/lib/utils'

import { DashCard } from './DashCard'

type Course = StudentGrades['courses'][number]
type Item =
  | { key: string; at: string; course: Course; letter: string }
  | { key: string; at: string; course: Course; component: Course['components'][number] }

/** latest, dönemin en son ilan edilen notlarıdır: ilan edilmiş harf notları ve yayımlanan bileşen notları. */
function latest(data: StudentGrades, limit: number): Item[] {
  const items: Item[] = data.courses.flatMap((c): Item[] =>
    c.grade_published_at && c.letter_grade
      ? [{ key: c.enrollment_id, at: c.grade_published_at, course: c, letter: c.letter_grade }]
      : c.components
          .filter((x) => x.published_at && (x.score !== null || x.absent))
          .map((x) => ({ key: `${c.enrollment_id}:${x.id}`, at: x.published_at!, course: c, component: x })),
  )
  return items.sort((a, b) => b.at.localeCompare(a.at)).slice(0, limit)
}

/**
 * GradesCard, öğrencinin içinde bulunulan dönemde son ilan edilen notlarıdır: harf notları ve
 * öğretim elemanının yayımladığı bileşen notları (sınıf ortalamasıyla).
 */
export function GradesCard() {
  const { t, i18n } = useTranslation()
  const localized = useLocalized()
  const en = i18n.language.startsWith('en')
  const programs = useListMyProgramsQuery()
  const sp = programs.data?.items[0]
  const { data, isError } = useGetStudentGradesQuery({ id: sp?.id ?? '' }, { skip: !sp })
  if (programs.isSuccess && !sp) {
    return null
  }
  const items = data ? latest(data, 4) : []
  const num = (v: number) => v.toLocaleString(en ? 'en-GB' : 'tr-TR', { maximumFractionDigits: 2 })
  return (
    <DashCard
      icon={BookOpenCheckIcon}
      tone="pink"
      title={t('dashboard.grades.title')}
      description={t('dashboard.grades.description')}
    >
      {!data && !isError ? (
        <Skeleton className="h-28" />
      ) : items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('dashboard.grades.empty')}</p>
      ) : (
        <ul className="grid grid-cols-[minmax(0,1fr)] gap-2">
          {items.map((it) => {
            const tone = toneFor(it.course.course.code)
            return (
              <li
                key={it.key}
                className="flex min-w-0 items-center gap-3 rounded-xl border border-border/70 px-3 py-2.5"
              >
                <span className={cn('rounded-md px-1.5 py-0.5 font-mono text-xs font-bold', toneSoft[tone])}>
                  {it.course.course.code}
                </span>
                <span className="min-w-0 flex-1 text-sm">
                  <span className="block truncate font-semibold">{localized(it.course.course)}</span>
                  <span className="text-xs text-muted-foreground">
                    {'letter' in it
                      ? t('dashboard.grades.letter')
                      : en
                        ? it.component.label_en
                        : it.component.label_tr}{' '}
                    · {formatRelative(it.at)}
                  </span>
                </span>
                {'letter' in it ? (
                  <LetterGrade letter={it.letter} className="min-w-11 py-1 text-sm" />
                ) : (
                  <span className="text-right">
                    <span className="block font-extrabold tabular-nums">
                      {it.component.absent ? t('grades.didNotSit') : num(it.component.score!)}
                    </span>
                    {it.component.class_average !== null && (
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {t('dashboard.grades.classAverage', { value: num(it.component.class_average) })}
                      </span>
                    )}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      )}
      <Button variant="soft" size="sm" className="mt-auto self-start" asChild>
        <Link to="/notlarim">
          {t('dashboard.grades.all')}
          <ArrowRightIcon />
        </Link>
      </Button>
    </DashCard>
  )
}
