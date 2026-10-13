import { ArrowLeftIcon, FlaskConicalIcon, PrinterIcon, RotateCcwIcon, XIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'

import { navTheme } from '@/features/shell/nav'
import {
  useGetTranscriptQuery,
  useListMyProgramsQuery,
  useSimulateTranscriptQuery,
  type TranscriptSimulation,
} from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Gpa } from '@/shared/ui/gpa'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { Spinner } from '@/shared/ui/spinner'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { TranscriptDocument } from './TranscriptDocument'

/**
 * TranscriptPage, transkriptin yazdırılabilir sayfasıdır. Yolda program kaydı varsa onun
 * transkripti (danışman ve öğrenci işleri öğrenci detayından gelir), yoksa öğrencinin kendi
 * programı (birden fazlaysa `?program=`). Tarayıcının yazdırma penceresiyle PDF'e çevrilebilir.
 *
 * "Ne olur?" modu sanal transkripttir: derslere varsayımsal harfler seçilir, YANO ve GANO
 * sunucuda aynı hesapla yeniden hesaplanır; hiçbir şey kaydedilmez.
 */
export function TranscriptPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { programId } = useParams()
  const [params] = useSearchParams()
  const mine = useListMyProgramsQuery(undefined, { skip: Boolean(programId) })
  const own = mine.data?.items.find((p) => p.id === params.get('program')) ?? mine.data?.items[0]
  const id = programId ?? own?.id
  const { data, error, refetch } = useGetTranscriptQuery({ id: id ?? '' }, { skip: !id })

  const [simulating, setSimulating] = useState(false)
  const [edits, setEdits] = useState<Record<string, string>>({})
  // Seçimler her değişiklikte değil, kısa bir duraksamadan sonra hesaplatılır.
  const [settled, setSettled] = useState<Record<string, string>>({})
  useEffect(() => {
    const timer = setTimeout(() => setSettled(edits), 300)
    return () => clearTimeout(timer)
  }, [edits])
  const grades = Object.entries(settled).map(([enrollment_id, letter_grade]) => ({
    enrollment_id,
    letter_grade,
  }))
  const simulation = useSimulateTranscriptQuery(
    { id: id ?? '', body: { grades } },
    { skip: !id || !simulating || grades.length === 0 },
  )
  // Seçim kalmayınca (sıfırla) gecikmeyi beklemeden gerçek değerlere dönülür.
  const result = grades.length > 0 && Object.keys(edits).length > 0 ? simulation.data : undefined

  if (error || mine.error) {
    return <ErrorState error={error ?? mine.error} onRetry={() => void refetch()} />
  }
  if (!programId && mine.data && !own) {
    return <EmptyState {...navTheme('/transkript')}>{t('transcript.noProgram')}</EmptyState>
  }
  const change = (enrollmentId: string, letter: string | null) =>
    setEdits((prev) => {
      const next = { ...prev }
      if (letter) {
        next[enrollmentId] = letter
      } else {
        delete next[enrollmentId]
      }
      return next
    })
  const close = () => {
    setSimulating(false)
    setEdits({})
  }

  return (
    <>
      {programId && (
        <Button
          variant="ghost"
          size="sm"
          className="mb-2 -ml-2 print:hidden"
          onClick={() => void navigate(-1)}
        >
          <ArrowLeftIcon />
          {t('common.back')}
        </Button>
      )}
      <PageHeader
        {...navTheme('/transkript')}
        className="print:hidden"
        title={simulating ? t('transcript.sim.title') : t('transcript.title')}
        description={simulating ? t('transcript.sim.hint') : t('transcript.description')}
        actions={
          <>
            {!programId && !simulating && mine.data && mine.data.items.length > 1 && (
              <div className="flex flex-wrap gap-1">
                {mine.data.items.map((p) => (
                  <Button key={p.id} asChild size="sm" variant={p.id === own?.id ? 'soft' : 'ghost'}>
                    <Link to={`?program=${p.id}`}>{p.program.code}</Link>
                  </Button>
                ))}
              </div>
            )}
            {simulating ? (
              <Button variant="outline" onClick={close}>
                <XIcon />
                {t('transcript.sim.close')}
              </Button>
            ) : (
              <>
                <Button variant="soft" onClick={() => setSimulating(true)} disabled={!data}>
                  <FlaskConicalIcon />
                  {t('transcript.sim.open')}
                </Button>
                <Button onClick={() => window.print()} disabled={!data}>
                  <PrinterIcon />
                  {t('transcript.print')}
                </Button>
              </>
            )}
          </>
        }
      />
      {simulating && data && (
        <SimulationBar
          actualGpa={data.summary.gpa}
          actualEcts={data.summary.earned_ects}
          result={result}
          changes={Object.keys(edits).length}
          loading={simulation.isFetching}
          failed={Boolean(simulation.error) && grades.length > 0}
          onReset={() => setEdits({})}
        />
      )}
      {data ? (
        <TranscriptDocument
          transcript={data}
          simulation={simulating ? { edits, onChange: change, result } : undefined}
        />
      ) : (
        <Skeleton className="h-96" />
      )}
    </>
  )
}

/** SimulationBar, sanal transkriptin özetidir: gerçek ve varsayımsal GANO ile kazanılan AKTS. */
function SimulationBar({
  actualGpa,
  actualEcts,
  result,
  changes,
  loading,
  failed,
  onReset,
}: {
  actualGpa: number | null
  actualEcts: number
  result?: TranscriptSimulation
  changes: number
  loading: boolean
  failed: boolean
  onReset: () => void
}) {
  const { t } = useTranslation()
  const gpa = result ? result.summary.simulated_gpa : actualGpa
  const ects = result ? result.summary.simulated_earned_ects : actualEcts
  const delta = gpa !== null && actualGpa !== null ? Math.round((gpa - actualGpa) * 100) / 100 : 0
  return (
    <div
      className="sticky top-20 z-10 mb-6 flex flex-wrap items-center justify-between gap-x-6 gap-y-2 rounded-2xl border border-violet-400/40 bg-violet-50/95 p-3 shadow-soft backdrop-blur sm:p-4 dark:bg-violet-950/80 print:hidden"
      role="status"
      aria-label={t('transcript.sim.title')}
    >
      <div className="flex w-full items-center gap-3 sm:w-auto">
        <IconChip icon={FlaskConicalIcon} tone="violet" solid className="hidden sm:inline-flex" />
        <div className="text-sm">
          <p className="font-bold">{t('transcript.sim.changes', { count: changes })}</p>
          <p className="hidden text-muted-foreground sm:block">
            {failed ? t('transcript.sim.failed') : t('transcript.sim.nothingSaved')}
          </p>
        </div>
      </div>
      <dl className="flex flex-wrap items-center gap-x-6 gap-y-1 sm:gap-x-8">
        <div>
          <dt className="text-xs font-semibold text-muted-foreground">{t('transcript.sim.gpa')}</dt>
          <dd className="flex items-baseline gap-2 text-xl font-extrabold sm:text-2xl">
            <Gpa value={gpa} />
            {delta !== 0 && (
              <span
                className={cn(
                  'text-sm font-bold tabular-nums',
                  delta > 0 ? 'text-emerald-700 dark:text-emerald-300' : 'text-rose-700 dark:text-rose-300',
                )}
              >
                {delta > 0 ? '+' : ''}
                {delta.toFixed(2)}
              </span>
            )}
            {loading && <Spinner className="size-4" />}
          </dd>
        </div>
        <div>
          <dt className="text-xs font-semibold text-muted-foreground">{t('transcript.sim.earned')}</dt>
          <dd className="text-xl font-extrabold text-sky-700 tabular-nums sm:text-2xl dark:text-sky-300">
            {ects}
            {ects !== actualEcts && (
              <span className="ml-1.5 text-sm font-bold text-muted-foreground">
                ({ects > actualEcts ? '+' : ''}
                {ects - actualEcts})
              </span>
            )}
          </dd>
        </div>
      </dl>
      <Button variant="ghost" size="sm" onClick={onReset} disabled={changes === 0}>
        <RotateCcwIcon />
        {t('transcript.sim.reset')}
      </Button>
    </div>
  )
}
