import {
  ArrowRightIcon,
  CircleAlertIcon,
  FileSpreadsheetIcon,
  MessageSquareTextIcon,
  RefreshCwIcon,
  UploadIcon,
  UserRoundXIcon,
} from 'lucide-react'
import { useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatNumber } from '@/i18n/format'
import { XLSX } from '@/shared/api/baseQuery'
import { ifMatch } from '@/shared/api/etag'
import {
  useImportAssignmentGradesMutation,
  usePreviewAssignmentGradeImportMutation,
  type AssignmentGradeImportPreview,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/shared/ui/dialog'
import { Spinner } from '@/shared/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

type Filter = 'changed' | 'errors' | 'all'

const statusVariant = {
  CHANGED: 'success',
  UNCHANGED: 'muted',
  EMPTY: 'outline',
  ERROR: 'destructive',
} as const

/** fileType, dosyanın uzantısına göre gönderilecek türdür; desteklenmiyorsa null. */
function fileType(file: File): string | null {
  const name = file.name.toLowerCase()
  if (name.endsWith('.xlsx')) {
    return XLSX
  }
  return name.endsWith('.csv') || name.endsWith('.txt') ? 'text/csv' : null
}

async function fileBlob(file: File, type: string): Promise<Blob> {
  return new Response(await file.arrayBuffer(), { headers: { 'Content-Type': type } }).blob()
}

/**
 * ImportGradesDialog, ödevin puanlarını ve geri bildirimlerini Excel şablonundan (ya da CSV'den)
 * içe aktarır: dosya önce satır satır önizlenir, hatasız dosya onaylanınca tek seferde yazılır.
 */
export function ImportGradesDialog({
  activityId,
  onTemplate,
  onClose,
}: {
  activityId: string
  onTemplate: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [file, setFile] = useState<File | null>(null)
  const [result, setResult] = useState<AssignmentGradeImportPreview | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const [stale, setStale] = useState(false)
  const [filter, setFilter] = useState<Filter>('changed')
  const [dragging, setDragging] = useState(false)
  const [preview, { isLoading: previewing }] = usePreviewAssignmentGradeImportMutation()
  const [apply, { isLoading: applying }] = useImportAssignmentGradesMutation()

  const run = async (f: File) => {
    const type = fileType(f)
    setResult(null)
    setStale(false)
    if (!type) {
      setProblem(t('lms.assignment.import.badType'))
      return
    }
    setProblem(null)
    const res = await preview({ id: activityId, body: await fileBlob(f, type) })
    if (res.error) {
      setProblem(message(res.error))
      return
    }
    setResult(res.data)
    setFilter(res.data.summary.errors > 0 ? 'errors' : 'changed')
  }

  const choose = (f: File | undefined) => {
    if (f) {
      setFile(f)
      void run(f)
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    choose(e.dataTransfer.files[0])
  }

  const onApply = async () => {
    const type = file && fileType(file)
    if (!file || !result || !type) {
      return
    }
    const res = await apply({
      id: activityId,
      'If-Match': ifMatch(result.version),
      body: await fileBlob(file, type),
    })
    if (res.error) {
      if ((res.error as { status?: unknown }).status === 412) {
        setStale(true)
        return
      }
      toast.error(message(res.error))
      return
    }
    toast.success(t('lms.assignment.import.done', { count: result.summary.changed }))
    onClose()
  }

  const rows = (result?.rows ?? []).filter((r) =>
    filter === 'all' ? true : filter === 'errors' ? r.status === 'ERROR' : r.status === 'CHANGED',
  )
  const score = (v: number | null) => (v === null ? '—' : formatNumber(v))

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-3xl" closeLabel={t('common.close')}>
        <DialogHeader icon={FileSpreadsheetIcon} tone="emerald">
          <DialogTitle>{t('lms.assignment.import.title')}</DialogTitle>
          <DialogDescription>{t('lms.assignment.import.description')}</DialogDescription>
        </DialogHeader>

        <div className="flex justify-end">
          <Button variant="outline" onClick={onTemplate}>
            <FileSpreadsheetIcon />
            {t('lms.assignment.import.template')}
          </Button>
        </div>

        <label
          onDragOver={(e) => {
            e.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className={cn(
            'flex cursor-pointer flex-col items-center gap-2 rounded-xl border-2 border-dashed border-emerald-400/50 bg-emerald-50/50 px-4 py-6 text-center transition-colors hover:bg-emerald-50 dark:bg-emerald-500/5',
            dragging && 'border-emerald-500 bg-emerald-100/60',
          )}
        >
          <UploadIcon className="size-6 text-emerald-600" aria-hidden />
          <span className="font-semibold">{file ? file.name : t('lms.assignment.import.drop')}</span>
          <span className="text-xs text-muted-foreground">{t('lms.assignment.import.formats')}</span>
          <input
            type="file"
            accept=".xlsx,.csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
            className="sr-only"
            aria-label={t('lms.assignment.import.file')}
            onChange={(e) => {
              choose(e.target.files?.[0])
              // Aynı dosya düzeltilip yeniden seçilebilsin.
              e.target.value = ''
            }}
          />
        </label>

        {previewing && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Spinner /> {t('lms.assignment.import.checking')}
          </p>
        )}
        {problem && (
          <p
            role="alert"
            className="flex items-start gap-2 rounded-xl bg-rose-50 px-3 py-2.5 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300"
          >
            <CircleAlertIcon className="mt-0.5 size-4 shrink-0" aria-hidden />
            {problem}
          </p>
        )}
        {stale && (
          <p
            role="alert"
            className="flex flex-wrap items-center gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-300"
          >
            <CircleAlertIcon className="size-4 shrink-0" aria-hidden />
            {t('lms.assignment.import.stale')}
            <Button size="sm" variant="outline" onClick={() => file && void run(file)}>
              <RefreshCwIcon />
              {t('lms.assignment.import.again')}
            </Button>
          </p>
        )}

        {result && (
          <section aria-label={t('lms.assignment.import.preview')} className="space-y-3">
            <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {(
                [
                  ['changed', 'border-emerald-300/60 bg-emerald-50 dark:bg-emerald-500/10'],
                  ['unchanged', 'border-border bg-muted/40'],
                  ['empty', 'border-sky-300/60 bg-sky-50 dark:bg-sky-500/10'],
                  ['errors', 'border-rose-300/60 bg-rose-50 dark:bg-rose-500/10'],
                ] as const
              ).map(([k, cls]) => (
                <div key={k} className={cn('rounded-xl border px-3 py-2', cls)}>
                  <dt className="text-xs font-semibold text-muted-foreground">
                    {t(`lms.assignment.import.summary.${k}`)}
                  </dt>
                  <dd className="text-xl font-bold tabular-nums">{result.summary[k]}</dd>
                </div>
              ))}
            </dl>
            <Tabs value={filter} onValueChange={(v) => setFilter(v as Filter)}>
              <TabsList>
                <TabsTrigger value="changed">{t('lms.assignment.import.filters.changed')}</TabsTrigger>
                <TabsTrigger value="errors">{t('lms.assignment.import.filters.errors')}</TabsTrigger>
                <TabsTrigger value="all">{t('lms.assignment.import.filters.all')}</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className="max-h-72 overflow-y-auto rounded-xl border">
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-muted text-left text-xs">
                  <tr>
                    <th scope="col" className="px-3 py-2">
                      {t('lms.assignment.import.line')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      {t('lms.assignment.teacher.columns.student')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      {t('lms.assignment.import.change')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      {t('lms.assignment.import.result')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="px-3 py-6 text-center text-muted-foreground">
                        {t('lms.assignment.import.noRows')}
                      </td>
                    </tr>
                  ) : (
                    rows.map((r) => (
                      <tr key={r.line} className="border-t">
                        <td className="px-3 py-2 text-muted-foreground tabular-nums">{r.line}</td>
                        <td className="px-3 py-2">
                          {r.student && (
                            <span className="font-semibold">
                              {r.student.first_name} {r.student.last_name}
                            </span>
                          )}
                          <span className="block font-mono text-xs text-muted-foreground">
                            {r.student_no || '—'}
                          </span>
                        </td>
                        <td className="px-3 py-2 tabular-nums">
                          {r.status === 'ERROR' ? (
                            <span className="text-rose-700 dark:text-rose-300">
                              {t(`lms.assignment.import.errors.${r.error_code ?? 'INVALID_SCORE'}`)}
                            </span>
                          ) : r.status === 'EMPTY' ? (
                            <span className="text-muted-foreground">
                              {t('lms.assignment.import.emptyCell')}
                            </span>
                          ) : (
                            <span className="flex flex-wrap items-center gap-1.5">
                              <span className="text-muted-foreground">{score(r.current_score)}</span>
                              <ArrowRightIcon className="size-3.5 text-muted-foreground" aria-hidden />
                              <strong>{score(r.next_score)}</strong>
                              {r.feedback_changed && (
                                <Badge variant="info" className="gap-1">
                                  <MessageSquareTextIcon className="size-3" />
                                  {t('lms.assignment.import.feedbackChanged')}
                                </Badge>
                              )}
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-2">
                          <Badge variant={statusVariant[r.status]}>
                            {t(`lms.assignment.import.status.${r.status}`)}
                          </Badge>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            {result.missing.length > 0 && (
              <details className="rounded-xl border bg-muted/30 px-3 py-2 text-sm">
                <summary className="flex cursor-pointer items-center gap-2 font-semibold">
                  <UserRoundXIcon className="size-4 text-muted-foreground" aria-hidden />
                  {t('lms.assignment.import.missing', { count: result.missing.length })}
                </summary>
                <p className="mt-2 text-muted-foreground">
                  {result.missing.map((s) => `${s.first_name} ${s.last_name} (${s.student_no})`).join(', ')}
                </p>
              </details>
            )}
          </section>
        )}

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => void onApply()}
            disabled={
              !result || result.summary.errors > 0 || result.summary.changed === 0 || applying || stale
            }
          >
            {applying ? <Spinner /> : <UploadIcon />}
            {result && result.summary.errors > 0
              ? t('lms.assignment.import.fixFirst')
              : t('lms.assignment.import.apply', { count: result?.summary.changed ?? 0 })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
