import {
  AwardIcon,
  ClipboardListIcon,
  DownloadIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  LockIcon,
  MegaphoneIcon,
  SaveIcon,
  ShieldAlertIcon,
  Undo2Icon,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useBlocker, useParams } from 'react-router'
import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { navTheme } from '@/features/shell/nav'
import { formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  agoraApi,
  useGetGradebookQuery,
  usePublishComponentScoresMutation,
  useSetAttendanceMutation,
  useSetComponentScoresMutation,
  type Gradebook,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { saveFile } from '@/shared/lib/download'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { componentColors } from './colors'
import { GradeTable } from './GradeTable'
import { ImportScoresDialog } from './ImportScoresDialog'
import { cellKey, parseCell, sameCell } from './scoreCell'

type Component = Gradebook['components'][number]

/**
 * GradebookPage, şubenin not tablosudur: öğretim elemanı bileşen notlarını girer (taslak →
 * kaydet), Excel şablonundan içe aktarır, bileşenleri öğrencilere yayımlar ve devamsızlığı
 * işaretler. Not girişi MFA'lı oturumda, plan kilitliyken ve bileşenin penceresi açıkken
 * olur; bölüm ve öğrenci işleri tabloyu sadece görür. Kaydedilmemiş değişiklik varken sayfadan
 * ayrılmak uyarı verir.
 */
export function GradebookPage() {
  const { t, i18n } = useTranslation()
  const { id = '' } = useParams()
  const localized = useLocalized()
  const message = useErrorMessage()
  const dispatch = useAppDispatch()
  // Veri girişi ekranı: her açılışta güncel tablo (ortak öğretim elemanının girdikleri, pencereler).
  const { data: book, error, refetch } = useGetGradebookQuery({ id }, { refetchOnMountOrArgChange: true })
  const [drafts, setDrafts] = useState(new Map<string, string>())
  const [saving, setSaving] = useState(false)
  const [importing, setImporting] = useState(false)
  const [publishing, setPublishing] = useState<Component | null>(null)
  const [setScores] = useSetComponentScoresMutation()
  const [publish] = usePublishComponentScoresMutation()
  const [setAttendance] = useSetAttendanceMutation()
  const en = i18n.language.startsWith('en')

  // Kaydedilmemiş (geçerli ya da geçersiz) taslaklar: değeri kayıtlı notla aynı olanlar sayılmaz.
  const pending = book ? pendingDrafts(book, drafts) : []
  const invalid = pending.filter((p) => p.value === null)
  const dirty = pending.length > 0

  const blocker = useBlocker(dirty)
  useEffect(() => {
    if (!dirty) {
      return
    }
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!book) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-96" />
        <Skeleton className="h-32" />
        <Skeleton className="h-96" />
      </div>
    )
  }

  const s = book.section
  const editable = (c: Component) => c.editable
  const editableComponents = book.components.filter(editable)
  const attendanceEditable = book.can_enter && s.plan_locked && !s.grades_published_at
  const label = (c: Component) => (en ? c.label_en : c.label_tr)
  const colorOf = componentColors(book.components)

  const onDraft = (key: string, text: string | undefined) =>
    setDrafts((prev) => {
      const next = new Map(prev)
      if (text === undefined) {
        next.delete(key)
      } else {
        next.set(key, text)
      }
      return next
    })

  const save = async () => {
    setSaving(true)
    let saved = 0
    const failed: string[] = []
    const done = new Set<string>()
    for (const c of editableComponents) {
      const items = pending
        .filter((p) => p.componentId === c.id && p.value)
        .map((p) => ({ enrollment_id: p.enrollmentId, score: p.value!.score, absent: p.value!.absent }))
      if (items.length === 0) {
        continue
      }
      const res = await setScores({ id: c.id, 'If-Match': ifMatch(c.version), body: { items } })
      if ('error' in res && res.error) {
        failed.push(`${label(c)}: ${message(res.error)}`)
        continue
      }
      saved += items.length
      done.add(c.id)
    }
    setDrafts((prev) => new Map([...prev].filter(([k]) => !done.has(k.split(':')[1]!))))
    setSaving(false)
    if (saved > 0) {
      toast.success(t('gradebook.saved', { count: saved }))
    }
    for (const f of failed) {
      toast.error(f)
    }
  }

  const download = async (format: 'xlsx' | 'csv') => {
    const lang = en ? 'en' : 'tr'
    const name = `${s.course.code}-${s.section_code}_${s.term.code}_${en ? 'grades' : 'notlar'}.${format}`
    try {
      if (format === 'xlsx') {
        const blob = await dispatch(
          agoraApi.endpoints.exportGradebookXlsx.initiate(
            { id, lang },
            { subscribe: false, forceRefetch: true },
          ),
        ).unwrap()
        saveFile(blob, name)
      } else {
        const text = await dispatch(
          agoraApi.endpoints.exportGradebook.initiate({ id, lang }, { subscribe: false, forceRefetch: true }),
        ).unwrap()
        saveFile(new Blob([text], { type: 'text/csv;charset=utf-8' }), name)
      }
    } catch (e) {
      toast.error(message(e as never))
    }
  }

  const onAttendance = async (enrollmentId: string, fail: boolean) => {
    const res = await setAttendance({
      id,
      body: {
        items: [{ enrollment_id: enrollmentId, attendance_status: fail ? 'ATTENDANCE_FAIL' : 'CONTINUING' }],
      },
    })
    if ('error' in res && res.error) {
      toast.error(message(res.error))
    }
  }

  return (
    <>
      <PageHeader
        {...navTheme('/verdigim-dersler')}
        title={t('gradebook.title')}
        description={`${s.course.code}-${s.section_code} ${localized(s.course)} · ${s.term.code}`}
        actions={
          <div className="flex flex-wrap gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline">
                  <DownloadIcon />
                  {t('gradebook.export')}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => void download('xlsx')}>
                  <FileSpreadsheetIcon />
                  {t('gradebook.exportXlsx')}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => void download('csv')}>
                  <FileTextIcon />
                  {t('gradebook.exportCsv')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            {editableComponents.length > 0 && (
              <Button variant="soft" onClick={() => setImporting(true)} disabled={dirty}>
                <FileSpreadsheetIcon />
                {t('gradebook.importExcel')}
              </Button>
            )}
            <Button variant="ghost" asChild>
              <Link to={`/subeler/${id}/degerlendirme`}>
                <ClipboardListIcon />
                {t('teaching.plan')}
              </Link>
            </Button>
            <Button variant="ghost" asChild>
              <Link to={`/subeler/${id}/harf-notlari`}>
                <AwardIcon />
                {t('gradebook.letters')}
              </Link>
            </Button>
          </div>
        }
      />

      <div className="mb-6 space-y-3">
        {!s.plan_locked && (
          <Alert>
            <LockIcon />
            <AlertDescription>
              {t('gradebook.planNotLocked')}{' '}
              <Link className="font-semibold underline" to={`/subeler/${id}/degerlendirme`}>
                {t('teaching.plan')}
              </Link>
            </AlertDescription>
          </Alert>
        )}
        {book.mfa_required && (
          <Alert variant="warning">
            <ShieldAlertIcon />
            <AlertDescription>{t('gradebook.mfaRequired')}</AlertDescription>
          </Alert>
        )}
        {s.instructor_role === 'ASSISTANT' && (
          <Alert>
            <AlertDescription>{t('gradebook.assistant')}</AlertDescription>
          </Alert>
        )}
        {s.grades_published_at && (
          <Alert variant="info">
            <MegaphoneIcon />
            <AlertDescription>
              {t('gradebook.published', { time: formatDateTime(s.grades_published_at) })}{' '}
              <Link className="font-semibold underline" to="/not-duzeltme">
                {t('gradebook.requestChange')}
              </Link>
            </AlertDescription>
          </Alert>
        )}
      </div>

      <ul className="mb-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label={t('gradebook.components')}>
        {book.components.map((c) => {
          const total = book.students.length
          return (
            <li
              key={c.id}
              className="relative flex flex-col gap-2 overflow-hidden rounded-xl border border-border/70 bg-card p-4 pt-5 shadow-xs"
            >
              <span aria-hidden className={cn('absolute inset-x-0 top-0 h-1.5', colorOf(c))} />
              <div className="flex items-start justify-between gap-2">
                <div>
                  <p className="font-bold">{label(c)}</p>
                  <p className="text-xs text-muted-foreground">%{c.weight}</p>
                </div>
                {c.published_at ? (
                  <Badge variant="success">{t('gradebook.componentPublished')}</Badge>
                ) : c.lms_fed ? (
                  <Badge variant="violet" title={t('gradebook.lmsFedHint')}>
                    {t('gradebook.lmsFed')}
                  </Badge>
                ) : c.editable ? (
                  <Badge variant="warning">{t('gradebook.editable')}</Badge>
                ) : (
                  <Badge variant="muted">{t('gradebook.closed')}</Badge>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                {c.entry_window.open && c.entry_window.ends_at
                  ? t('gradebook.windowOpen', { time: formatDateTime(c.entry_window.ends_at) })
                  : c.entry_window.next_starts_at
                    ? t('gradebook.windowNext', { time: formatDateTime(c.entry_window.next_starts_at) })
                    : t('gradebook.windowClosed')}
              </p>
              <p className="text-sm tabular-nums">
                {t('gradebook.entered', { count: c.scored_count + c.absent_count, total })}
                {c.absent_count > 0 && ` · ${t('gradebook.absentCount', { count: c.absent_count })}`}
                {c.average !== null &&
                  ` · ${t('gradebook.average', { value: c.average.toLocaleString(en ? 'en-GB' : 'tr-TR') })}`}
              </p>
              {book.can_enter && !c.published_at && c.scored_count + c.absent_count > 0 && (
                <Button
                  size="sm"
                  variant="soft"
                  className="mt-auto self-start"
                  onClick={() => setPublishing(c)}
                >
                  <MegaphoneIcon />
                  {t('gradebook.publish')}
                </Button>
              )}
            </li>
          )
        })}
      </ul>

      {book.students.length === 0 ? (
        <EmptyState icon={ClipboardListIcon} tone="fuchsia">
          {t('gradebook.empty')}
        </EmptyState>
      ) : (
        <GradeTable
          book={book}
          drafts={drafts}
          editable={editable}
          onDraft={onDraft}
          attendanceEditable={attendanceEditable}
          onAttendance={(e, fail) => void onAttendance(e, fail)}
        />
      )}
      <p className="mt-3 text-xs text-muted-foreground">{t('gradebook.keyboard')}</p>

      {dirty && (
        <div
          role="region"
          aria-label={t('gradebook.unsavedRegion')}
          className="sticky bottom-4 z-30 mt-4 flex flex-wrap items-center gap-3 rounded-2xl border border-amber-300/70 bg-amber-50/95 px-4 py-3 shadow-lift backdrop-blur dark:bg-amber-950/80"
        >
          <p className="min-w-0 flex-1 text-sm font-semibold">
            {t('gradebook.unsaved', { count: pending.length })}
            {invalid.length > 0 && (
              <span className="ml-2 font-normal text-rose-700 dark:text-rose-300">
                {t('gradebook.invalid', { count: invalid.length })}
              </span>
            )}
          </p>
          <Button variant="ghost" onClick={() => setDrafts(new Map())} disabled={saving}>
            <Undo2Icon />
            {t('gradebook.discard')}
          </Button>
          <Button onClick={() => void save()} disabled={saving || invalid.length > 0}>
            <SaveIcon />
            {t('gradebook.save')}
          </Button>
        </div>
      )}

      {importing && (
        <ImportScoresDialog
          components={editableComponents}
          onTemplate={() => void download('xlsx')}
          onClose={() => setImporting(false)}
        />
      )}
      {publishing && (
        <ConfirmDialog
          title={t('gradebook.publishTitle', { component: label(publishing) })}
          description={t('gradebook.publishDescription')}
          confirm={t('gradebook.publish')}
          icon={MegaphoneIcon}
          tone="emerald"
          onClose={() => setPublishing(null)}
          onConfirm={async () => {
            const res = await publish({ id: publishing.id })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return
            }
            toast.success(t('gradebook.publishedToast', { component: label(publishing) }))
            setPublishing(null)
          }}
        />
      )}
      {blocker.state === 'blocked' && (
        <ConfirmDialog
          title={t('gradebook.leaveTitle')}
          description={t('gradebook.leaveDescription', { count: pending.length })}
          confirm={t('gradebook.leave')}
          destructive
          onClose={() => blocker.reset()}
          onConfirm={() => blocker.proceed()}
        />
      )}
    </>
  )
}

type Pending = { key: string; enrollmentId: string; componentId: string; value: ReturnType<typeof parseCell> }

/** pendingDrafts, kayıtlı nottan farklı taslaklardır; geçersiz taslağın value'su null. */
function pendingDrafts(book: Gradebook, drafts: Map<string, string>): Pending[] {
  const out: Pending[] = []
  for (const st of book.students) {
    for (const c of book.components) {
      const key = cellKey(st.enrollment_id, c.id)
      const text = drafts.get(key)
      if (text === undefined || !c.editable || (c.type.category === 'MAKEUP' && !st.makeup_eligible)) {
        continue
      }
      const value = parseCell(text)
      if (value && sameCell(value, st.scores[c.id])) {
        continue
      }
      out.push({ key, enrollmentId: st.enrollment_id, componentId: c.id, value })
    }
  }
  return out
}
