import {
  ArrowRightIcon,
  CircleAlertIcon,
  FileSpreadsheetIcon,
  RefreshCwIcon,
  UploadIcon,
  UserRoundXIcon,
} from 'lucide-react'
import { useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ifMatch } from '@/shared/api/etag'
import { XLSX } from '@/shared/api/baseQuery'
import {
  useImportScoresMutation,
  usePreviewScoreImportMutation,
  type ComponentScore,
  type Gradebook,
  type ScoreImportPreview,
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
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Spinner } from '@/shared/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

type Component = Gradebook['components'][number]
type Filter = 'changed' | 'errors' | 'all'

const statusVariant = {
  CHANGED: 'success',
  UNCHANGED: 'muted',
  EMPTY: 'outline',
  ERROR: 'destructive',
} as const

/** fileType, dosyanın uzantısına göre gönderilecek türdür (Windows CSV'ye Excel türü verir); desteklenmiyorsa null. */
function fileType(file: File): string | null {
  const name = file.name.toLowerCase()
  if (name.endsWith('.xlsx')) {
    return XLSX
  }
  return name.endsWith('.csv') || name.endsWith('.txt') ? 'text/csv' : null
}

/** fileBlob, dosyayı doğru türle gönderilecek gövdeye çevirir. */
async function fileBlob(file: File, type: string): Promise<Blob> {
  return new Response(await file.arrayBuffer(), { headers: { 'Content-Type': type } }).blob()
}

/**
 * ImportScoresDialog, bir bileşenin notlarını Excel şablonundan (ya da CSV'den) içe aktarır:
 * dosya önce sunucuda satır satır doğrulanır ve önizlenir, hatasız dosya onaylanınca notlar
 * tek seferde yazılır. Önizlemeden sonra notlar başka yoldan değiştiyse dosya yeniden önizlenir.
 */
export function ImportScoresDialog({
  components,
  initial,
  onTemplate,
  onClose,
}: {
  components: Component[]
  initial?: string
  onTemplate: () => void
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const message = useErrorMessage()
  const en = i18n.language.startsWith('en')
  const [componentId, setComponentId] = useState(initial ?? components[0]?.id ?? '')
  const [file, setFile] = useState<File | null>(null)
  const [result, setResult] = useState<ScoreImportPreview | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const [stale, setStale] = useState(false)
  const [filter, setFilter] = useState<Filter>('changed')
  const [dragging, setDragging] = useState(false)
  const [preview, { isLoading: previewing }] = usePreviewScoreImportMutation()
  const [apply, { isLoading: applying }] = useImportScoresMutation()
  const component = components.find((c) => c.id === componentId)
  const label = (c: Component) => (en ? c.label_en : c.label_tr)

  const run = async (f: File, id: string) => {
    const type = fileType(f)
    setResult(null)
    setStale(false)
    if (!type) {
      setProblem(t('gradebook.import.badType'))
      return
    }
    setProblem(null)
    const res = await preview({ id, body: await fileBlob(f, type) })
    if ('error' in res && res.error) {
      setProblem(message(res.error))
      return
    }
    setResult(res.data)
    setFilter(res.data.summary.errors > 0 ? 'errors' : 'changed')
  }

  const choose = (f: File | undefined) => {
    if (f) {
      setFile(f)
      void run(f, componentId)
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    choose(e.dataTransfer.files[0])
  }

  const onApply = async () => {
    if (!file || !result) {
      return
    }
    const type = fileType(file)
    if (!type) {
      return
    }
    const res = await apply({
      id: componentId,
      'If-Match': ifMatch(result.version),
      body: await fileBlob(file, type),
    })
    if ('error' in res && res.error) {
      const status = (res.error as { status?: unknown }).status
      if (status === 412) {
        setStale(true)
        return
      }
      toast.error(message(res.error))
      return
    }
    toast.success(
      t('gradebook.import.done', {
        count: result.summary.changed,
        component: component ? label(component) : '',
      }),
    )
    onClose()
  }

  const rows = (result?.rows ?? []).filter((r) =>
    filter === 'all' ? true : filter === 'errors' ? r.status === 'ERROR' : r.status === 'CHANGED',
  )
  const score = (v: ComponentScore | null) =>
    !v ? '—' : v.absent ? t('gradebook.absentShort') : String(v.score).replace('.', en ? '.' : ',')

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-3xl" closeLabel={t('common.close')}>
        <DialogHeader icon={FileSpreadsheetIcon} tone="emerald">
          <DialogTitle>{t('gradebook.import.title')}</DialogTitle>
          <DialogDescription>{t('gradebook.import.description')}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 sm:grid-cols-[1fr_auto] sm:items-end">
          <div className="space-y-1.5">
            <Label htmlFor="import-component">{t('gradebook.import.component')}</Label>
            <Select
              value={componentId}
              onValueChange={(v) => {
                setComponentId(v)
                if (file) {
                  void run(file, v)
                }
              }}
            >
              <SelectTrigger id="import-component" aria-label={t('gradebook.import.component')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {components.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {label(c)} · %{c.weight}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button variant="outline" onClick={onTemplate}>
            <FileSpreadsheetIcon />
            {t('gradebook.import.template')}
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
          <span className="font-semibold">{file ? file.name : t('gradebook.import.drop')}</span>
          <span className="text-xs text-muted-foreground">{t('gradebook.import.formats')}</span>
          <input
            type="file"
            accept=".xlsx,.csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
            className="sr-only"
            aria-label={t('gradebook.import.file')}
            onChange={(e) => {
              choose(e.target.files?.[0])
              // Aynı dosya düzeltilip yeniden seçilebilsin: tarayıcı aynı değerde change üretmez.
              e.target.value = ''
            }}
          />
        </label>

        {previewing && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Spinner /> {t('gradebook.import.checking')}
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
            {t('gradebook.import.stale')}
            <Button size="sm" variant="outline" onClick={() => file && void run(file, componentId)}>
              <RefreshCwIcon />
              {t('gradebook.import.again')}
            </Button>
          </p>
        )}

        {result && (
          <section aria-label={t('gradebook.import.preview')} className="space-y-3">
            <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {(
                [
                  ['changed', 'emerald'],
                  ['unchanged', 'slate'],
                  ['empty', 'sky'],
                  ['errors', 'rose'],
                ] as const
              ).map(([k, tone]) => (
                <div
                  key={k}
                  className={cn(
                    'rounded-xl border px-3 py-2',
                    tone === 'emerald' && 'border-emerald-300/60 bg-emerald-50 dark:bg-emerald-500/10',
                    tone === 'slate' && 'border-border bg-muted/40',
                    tone === 'sky' && 'border-sky-300/60 bg-sky-50 dark:bg-sky-500/10',
                    tone === 'rose' && 'border-rose-300/60 bg-rose-50 dark:bg-rose-500/10',
                  )}
                >
                  <dt className="text-xs font-semibold text-muted-foreground">
                    {t(`gradebook.import.summary.${k}`)}
                  </dt>
                  <dd className="text-xl font-bold tabular-nums">{result.summary[k]}</dd>
                </div>
              ))}
            </dl>
            {result.column && (
              <p className="text-xs text-muted-foreground">
                {t('gradebook.import.column', { column: result.column })}
              </p>
            )}
            <Tabs value={filter} onValueChange={(v) => setFilter(v as Filter)}>
              <TabsList>
                <TabsTrigger value="changed">{t('gradebook.import.filters.changed')}</TabsTrigger>
                <TabsTrigger value="errors">{t('gradebook.import.filters.errors')}</TabsTrigger>
                <TabsTrigger value="all">{t('gradebook.import.filters.all')}</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className="max-h-72 overflow-y-auto rounded-xl border">
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-muted text-left text-xs">
                  <tr>
                    <th scope="col" className="px-3 py-2">
                      {t('gradebook.import.line')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      {t('gradebook.student')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      {t('gradebook.import.change')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      {t('gradebook.import.result')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="px-3 py-6 text-center text-muted-foreground">
                        {t('gradebook.import.noRows')}
                      </td>
                    </tr>
                  ) : (
                    rows.map((r) => (
                      <tr key={r.line} className="border-t">
                        <td className="px-3 py-2 text-muted-foreground tabular-nums">{r.line}</td>
                        <td className="px-3 py-2">
                          {r.student ? (
                            <span className="font-semibold">
                              {r.student.first_name} {r.student.last_name}
                            </span>
                          ) : null}
                          <span className="block font-mono text-xs text-muted-foreground">
                            {r.student_no || '—'}
                          </span>
                        </td>
                        <td className="px-3 py-2 tabular-nums">
                          {r.status === 'ERROR' ? (
                            <span className="text-rose-700 dark:text-rose-300">
                              {t(`gradebook.import.errors.${r.error_code ?? 'INVALID_SCORE'}`)}
                            </span>
                          ) : r.status === 'EMPTY' ? (
                            <span className="text-muted-foreground">{t('gradebook.import.emptyCell')}</span>
                          ) : (
                            <span className="flex items-center gap-1.5">
                              <span className="text-muted-foreground">{score(r.current)}</span>
                              <ArrowRightIcon className="size-3.5 text-muted-foreground" aria-hidden />
                              <strong>{score(r.next)}</strong>
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-2">
                          <Badge variant={statusVariant[r.status]}>
                            {t(`gradebook.import.status.${r.status}`)}
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
                  {t('gradebook.import.missing', { count: result.missing.length })}
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
              ? t('gradebook.import.fixFirst')
              : t('gradebook.import.apply', { count: result?.summary.changed ?? 0 })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
