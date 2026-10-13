import type { TFunction } from 'i18next'
import {
  AwardIcon,
  BookCheckIcon,
  GraduationCapIcon,
  HourglassIcon,
  RepeatIcon,
  TrendingUpIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { formatDateTime } from '@/i18n/format'
import type { TermResult, Transcript, TranscriptSimulation } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Brand } from '@/shared/ui/brand'
import { Gpa } from '@/shared/ui/gpa'
import { IconChip } from '@/shared/ui/icon-chip'
import { LetterGrade } from '@/shared/ui/letter-grade'
import { Meter } from '@/shared/ui/meter'
import { StatTile } from '@/shared/ui/stat-tile'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

type Term = Transcript['terms'][number]

/** Simulation, sanal transkript modudur: derslere verilen varsayımsal harfler ve son hesap. */
export type Simulation = {
  /** edits, kesin kayıt → varsayımsal harf. */
  edits: Record<string, string>
  onChange: (enrollmentId: string, letter: string | null) => void
  result?: TranscriptSimulation
}

/** termName, "2025-FALL" kodlu dönemi "2025-2026 Güz" biçiminde yazar. */
function termName(t: TFunction, term: Term['term']): string {
  const year = Number(term.code.slice(0, 4))
  const type = t(`calendar.termTypes.${term.term_type}`)
  return Number.isFinite(year) ? `${year}-${year + 1} ${type}` : `${term.code} ${type}`
}

/**
 * TranscriptDocument, program kaydının transkriptidir: öğrenci ve program bilgisi, genel özet,
 * dönem dönem dersler (tekrar ve geçersiz deneme işaretleriyle) ve dönem sonuçları. Ekranda
 * renkli, yazdırırken sayfa sayfa bölünecek biçimdedir.
 */
export function TranscriptDocument({
  transcript: tr,
  simulation,
}: {
  transcript: Transcript
  simulation?: Simulation
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const sp = tr.student_program
  const s = tr.summary
  const advisor = sp.advisor
    ? [sp.advisor.title, sp.advisor.first_name, sp.advisor.last_name].filter(Boolean).join(' ')
    : '—'

  return (
    <article className="space-y-6" aria-label={t('transcript.title')}>
      <header className="rounded-2xl border bg-card p-5 shadow-xs print:rounded-none print:border-0 print:p-0 print:shadow-none">
        <div className="flex items-start justify-between gap-4 border-b pb-4">
          <div className="flex items-center gap-3">
            <IconChip icon={GraduationCapIcon} tone="lime" size="lg" solid className="print:hidden" />
            <div>
              <h2 className="text-xl font-extrabold tracking-tight">
                {sp.student.first_name} {sp.student.last_name}
              </h2>
              <p className="text-sm text-muted-foreground">
                {sp.student.student_no} · {sp.program.name_tr}
              </p>
            </div>
          </div>
          <Brand className="hidden sm:flex print:flex" />
        </div>
        <dl className="mt-4 grid gap-x-8 gap-y-1.5 text-sm sm:grid-cols-2">
          {(
            [
              [t('transcript.faculty'), sp.program.faculty.name_tr],
              [t('transcript.department'), sp.program.department.name_tr],
              [t('transcript.program'), `${sp.program.name_tr} (${tr.program_name_en})`],
              [t('transcript.degree'), t(`transcript.degrees.${tr.degree_level}`)],
              [t('transcript.admission'), String(sp.admission_year)],
              [
                t('transcript.semester'),
                t('transcript.semesterValue', { semester: sp.current_semester, level: sp.class_level }),
              ],
              [t('transcript.status'), t(`students.status.${sp.status}`)],
              [t('transcript.advisor'), advisor],
            ] as const
          ).map(([label, value]) => (
            <div key={label} className="flex justify-between gap-4 border-b border-dashed py-1">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="text-right font-medium">{value}</dd>
            </div>
          ))}
        </dl>
      </header>

      <dl className="grid grid-cols-2 gap-3 lg:grid-cols-4 print:grid-cols-4">
        <StatTile
          label={t('transcript.gpa')}
          value={<Gpa value={s.gpa} />}
          icon={TrendingUpIcon}
          tone="emerald"
        />
        <div className="rounded-xl border border-border/70 bg-card p-3 shadow-xs">
          <dt className="text-xs font-semibold text-muted-foreground">{t('transcript.earned')}</dt>
          <dd className="mt-0.5 text-lg font-extrabold text-sky-700 tabular-nums dark:text-sky-300">
            {s.earned_ects}
            <span className="text-xs font-semibold text-muted-foreground"> / {s.required_ects} AKTS</span>
          </dd>
          <Meter
            value={s.earned_ects}
            max={s.required_ects}
            tone="sky"
            size="sm"
            className="mt-1.5"
            label={t('transcript.earned')}
          />
        </div>
        <StatTile
          label={t('transcript.completed')}
          value={s.completed_courses}
          suffix={s.in_progress > 0 ? t('transcript.inProgressCount', { count: s.in_progress }) : undefined}
          icon={BookCheckIcon}
          tone="violet"
        />
        <StatTile
          label={t('transcript.honors')}
          value={s.honor_terms + s.high_honor_terms}
          suffix={
            s.high_honor_terms > 0 ? t('transcript.highHonorCount', { count: s.high_honor_terms }) : undefined
          }
          icon={AwardIcon}
          tone="amber"
        />
      </dl>

      {tr.terms.length === 0 ? (
        <p className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('transcript.empty')}
        </p>
      ) : (
        tr.terms.map((term) => (
          <TermSection key={term.term.id} term={term} localized={localized} simulation={simulation} />
        ))
      )}

      <footer className="space-y-1 border-t pt-4 text-xs text-muted-foreground">
        <p className="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span className="inline-flex items-center gap-1">
            <RepeatIcon className="size-3.5 text-violet-600" aria-hidden />
            {t('transcript.legendRepeat')}
          </span>
          <span>
            <span className="font-mono line-through">F2</span> {t('transcript.legendSuperseded')}
          </span>
        </p>
        <p>{t('transcript.disclaimer', { time: formatDateTime(tr.generated_at) })}</p>
      </footer>
    </article>
  )
}

/** Compare, gerçek değeri ve sanal transkriptte farklıysa varsayımsal değeri gösterir. */
function Compare({
  actual,
  simulated,
  gpa = true,
}: {
  actual: number | null | undefined
  simulated?: number | null
  gpa?: boolean
}) {
  const value = (v: number | null | undefined) =>
    gpa ? <Gpa value={v} /> : <span className="font-semibold tabular-nums">{v ?? '—'}</span>
  if (simulated === undefined || simulated === actual) {
    return value(actual)
  }
  return (
    <span className="inline-flex items-center gap-1">
      <span className="opacity-60">{value(actual)}</span>
      <span aria-hidden>→</span>
      <span className="rounded-md bg-violet-500/12 px-1 ring-1 ring-violet-500/30">{value(simulated)}</span>
    </span>
  )
}

function TermSection({
  term,
  localized,
  simulation,
}: {
  term: Term
  localized: ReturnType<typeof useLocalized>
  simulation?: Simulation
}) {
  const { t } = useTranslation()
  const simulated: TermResult | null | undefined = simulation?.result?.terms.find(
    (x) => x.term.id === term.term.id,
  )?.simulated
  const r = term.result ?? simulated ?? null
  // shown, başlıkta gösterilen sonuç: sanal transkriptte varsayımsal, yoksa gerçek.
  const shown = simulated ?? r
  return (
    <section
      className="break-inside-avoid overflow-hidden rounded-2xl border bg-card shadow-xs print:rounded-none print:shadow-none"
      aria-label={termName(t, term.term)}
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/40 px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-bold">{termName(t, term.term)}</h3>
          {shown && shown.standing !== 'NORMAL' && (
            <Badge variant={shown.standing === 'HIGH_HONOR' ? 'violet' : 'warning'} className="gap-1">
              <AwardIcon className="size-3.5" aria-hidden />
              {t(`transcript.standing.${shown.standing}`)}
            </Badge>
          )}
          {!term.result && simulated && <Badge variant="violet">{t('transcript.sim.badge')}</Badge>}
        </div>
        {r ? (
          <dl className="flex flex-wrap gap-x-5 gap-y-1 text-sm">
            <div className="flex gap-1.5">
              <dt className="text-muted-foreground">{t('transcript.termGpa')}</dt>
              <dd>
                <Compare actual={term.result?.term_gpa} simulated={simulated?.term_gpa} />
              </dd>
            </div>
            <div className="flex gap-1.5">
              <dt className="text-muted-foreground">{t('transcript.gpa')}</dt>
              <dd>
                <Compare actual={term.result?.cumulative_gpa} simulated={simulated?.cumulative_gpa} />
              </dd>
            </div>
            <div className="flex gap-1.5">
              <dt className="text-muted-foreground">{t('transcript.termEcts')}</dt>
              <dd className="font-semibold tabular-nums">
                {(shown ?? r).earned_ects} / {(shown ?? r).attempted_ects}
              </dd>
            </div>
            <div className="flex gap-1.5">
              <dt className="text-muted-foreground">{t('transcript.cumulativeEcts')}</dt>
              <dd>
                <Compare
                  gpa={false}
                  actual={term.result?.cumulative_ects}
                  simulated={simulated?.cumulative_ects}
                />
              </dd>
            </div>
          </dl>
        ) : (
          <Badge variant="muted" className="gap-1">
            <HourglassIcon className="size-3.5" aria-hidden />
            {t('transcript.noResult')}
          </Badge>
        )}
      </div>
      <Table aria-label={t('transcript.coursesOf', { term: termName(t, term.term) })}>
        <TableHeader>
          <TableRow>
            <TableHead className="hidden w-28 sm:table-cell print:table-cell">{t('catalog.code')}</TableHead>
            <TableHead>{t('catalog.name')}</TableHead>
            <TableHead className="hidden w-16 text-right sm:table-cell print:table-cell">
              {t('catalog.credit')}
            </TableHead>
            <TableHead className="w-14 text-right sm:w-16">{t('catalog.ects')}</TableHead>
            <TableHead className="w-20 text-center sm:w-24">{t('transcript.letter')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {term.courses.map((c) => (
            <TableRow
              key={c.enrollment_id}
              className={cn(
                c.superseded && 'text-muted-foreground',
                simulation?.edits[c.enrollment_id] && 'bg-violet-500/6',
              )}
            >
              <TableCell className="hidden font-mono text-xs sm:table-cell print:table-cell">
                {c.course.code}
              </TableCell>
              <TableCell className="whitespace-normal">
                {/* Dar ekranda kod adın üstünde; kredi sütunu gizli. */}
                <span className="block font-mono text-[11px] text-muted-foreground sm:hidden print:hidden">
                  {c.course.code}
                </span>
                <span className="inline-flex flex-wrap items-center gap-1.5">
                  {localized(c.course)}
                  {c.is_repeat && (
                    <span title={t('transcript.legendRepeat')} className="inline-flex">
                      <RepeatIcon className="size-3.5 text-violet-600" aria-hidden />
                      <span className="sr-only">{t('transcript.repeat')}</span>
                    </span>
                  )}
                  {c.superseded && <span className="sr-only">{t('transcript.legendSuperseded')}</span>}
                </span>
              </TableCell>
              <TableCell className="hidden text-right tabular-nums sm:table-cell print:table-cell">
                {c.credit}
              </TableCell>
              <TableCell className="text-right tabular-nums">{c.ects}</TableCell>
              <TableCell className="text-center">
                {simulation ? (
                  <select
                    aria-label={t('transcript.sim.choose', { course: c.course.code })}
                    value={simulation.edits[c.enrollment_id] ?? ''}
                    onChange={(e) => simulation.onChange(c.enrollment_id, e.target.value || null)}
                    className={cn(
                      'h-8 w-full max-w-24 min-w-16 cursor-pointer rounded-lg border border-input bg-card px-1.5 font-mono text-xs font-bold shadow-xs outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
                      simulation.edits[c.enrollment_id] &&
                        'border-violet-500 text-violet-700 ring-2 ring-violet-500/25 dark:text-violet-300',
                    )}
                  >
                    {/* Boş seçenek gerçek harftir (süren derste henüz yok). */}
                    <option value="">{c.letter_grade ?? '—'}</option>
                    {c.letter_options.map((l) => (
                      <option key={l} value={l}>
                        {l}
                      </option>
                    ))}
                  </select>
                ) : c.letter_grade ? (
                  <LetterGrade
                    letter={c.letter_grade}
                    className={cn(c.superseded && 'line-through opacity-60')}
                  />
                ) : (
                  <Badge variant="info">{t('transcript.ongoing')}</Badge>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </section>
  )
}
