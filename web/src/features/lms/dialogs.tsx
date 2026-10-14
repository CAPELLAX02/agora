import { CircleAlertIcon, FileIcon, UploadIcon, XIcon } from 'lucide-react'
import { useState, type DragEvent, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import type { Activity, Topic } from '@/shared/api/generated'
import { formatBytes, UploadError, useUpload } from '@/shared/api/upload'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { RichTextEditor } from '@/shared/ui/rich-text-editor'
import { Spinner } from '@/shared/ui/spinner'

import { activityMeta, fromLocalInput, toLocalInput, type EditableType } from './meta'

type UploadedFile = { id: string; name: string; size: number }
type Pending = { key: string; name: string; progress: number; error?: string }

/**
 * FileDropzone, dosyaları sürükle-bırak ya da seçiciyle yükler: her dosya hemen ve ilerleme
 * çubuğuyla yüklenir, sunucunun reddi (tür, içerik, boyut) dosyanın yanında yazar.
 */
export function FileDropzone({
  files,
  onChange,
  multiple,
}: {
  files: UploadedFile[]
  onChange: (files: UploadedFile[]) => void
  multiple: boolean
}) {
  const { t, i18n } = useTranslation()
  const upload = useUpload()
  const [pending, setPending] = useState<Pending[]>([])
  const [dragging, setDragging] = useState(false)
  const locale = i18n.language.startsWith('en') ? 'en-GB' : 'tr-TR'

  const add = (list: FileList | null) => {
    const chosen = Array.from(list ?? []).slice(0, multiple ? 50 : 1)
    let current = multiple ? files : []
    if (!multiple) {
      onChange([])
    }
    for (const file of chosen) {
      const key = `${file.name}-${file.size}-${Math.random()}`
      setPending((p) => [...p, { key, name: file.name, progress: 0 }])
      upload(file, (loaded, total) =>
        setPending((p) => p.map((x) => (x.key === key ? { ...x, progress: total ? loaded / total : 0 } : x))),
      )
        .then((info) => {
          setPending((p) => p.filter((x) => x.key !== key))
          current = [...current, { id: info.id, name: info.name, size: info.size }]
          onChange(current)
        })
        .catch((err: unknown) => {
          const message =
            err instanceof UploadError && err.code !== 'NETWORK'
              ? err.message
              : err instanceof UploadError
                ? t('errors.network')
                : t('lms.upload.failed')
          setPending((p) => p.map((x) => (x.key === key ? { ...x, error: message, progress: 0 } : x)))
        })
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    add(e.dataTransfer.files)
  }

  return (
    <div className="space-y-2">
      <label
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          'flex cursor-pointer flex-col items-center gap-1.5 rounded-xl border-2 border-dashed border-sky-400/50 bg-sky-50/50 px-4 py-5 text-center transition-colors hover:bg-sky-50 dark:bg-sky-500/5',
          dragging && 'border-sky-500 bg-sky-100/60',
        )}
      >
        <UploadIcon className="size-5 text-sky-600" aria-hidden />
        <span className="text-sm font-semibold">
          {t(multiple ? 'lms.upload.dropMany' : 'lms.upload.dropOne')}
        </span>
        <span className="text-xs text-muted-foreground">{t('lms.upload.limits')}</span>
        <input
          type="file"
          multiple={multiple}
          className="sr-only"
          aria-label={t('lms.upload.choose')}
          onChange={(e) => {
            add(e.target.files)
            e.target.value = ''
          }}
        />
      </label>
      <ul className="space-y-1.5">
        {files.map((f) => (
          <li key={f.id} className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm">
            <FileIcon className="size-4 shrink-0 text-sky-600" aria-hidden />
            <span className="min-w-0 flex-1 truncate">{f.name}</span>
            <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(f.size, locale)}</span>
            <button
              type="button"
              aria-label={t('lms.upload.remove', { name: f.name })}
              onClick={() => onChange(files.filter((x) => x.id !== f.id))}
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <XIcon className="size-3.5" />
            </button>
          </li>
        ))}
        {pending.map((p) => (
          <li key={p.key} className="space-y-1 rounded-lg border px-3 py-2 text-sm" aria-live="polite">
            <span className="flex items-center gap-2">
              {p.error ? (
                <CircleAlertIcon className="size-4 shrink-0 text-rose-600" aria-hidden />
              ) : (
                <Spinner className="size-4" />
              )}
              <span className="min-w-0 flex-1 truncate">{p.name}</span>
              {p.error && (
                <button
                  type="button"
                  aria-label={t('lms.upload.dismiss')}
                  onClick={() => setPending((list) => list.filter((x) => x.key !== p.key))}
                  className="rounded p-1 text-muted-foreground hover:bg-muted"
                >
                  <XIcon className="size-3.5" />
                </button>
              )}
            </span>
            {p.error ? (
              <span className="block text-xs text-rose-700 dark:text-rose-300">{p.error}</span>
            ) : (
              <span
                role="progressbar"
                aria-label={p.name}
                aria-valuenow={Math.round(p.progress * 100)}
                aria-valuemin={0}
                aria-valuemax={100}
                className="block h-1.5 overflow-hidden rounded-full bg-muted"
              >
                <span
                  className="block h-full bg-sky-500 transition-all"
                  style={{ width: `${p.progress * 100}%` }}
                />
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

/** EditorDialog, ders içeriği düzenleme pencerelerinin ortak iskeletidir (geniş, kaydırılabilir). */
function EditorDialog({
  title,
  description,
  icon,
  error,
  busy,
  submit,
  onClose,
  onSubmit,
  children,
}: {
  title: string
  description?: string
  icon: Parameters<typeof DialogHeader>[0]['icon']
  error: string | null
  busy: boolean
  submit: string
  onClose: () => void
  onSubmit: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl" closeLabel={t('common.close')}>
        <DialogHeader icon={icon} tone="indigo">
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        <form
          noValidate
          className="space-y-4"
          onSubmit={(e: FormEvent) => {
            e.preventDefault()
            onSubmit()
          }}
        >
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {children}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={busy}>
              {busy && <Spinner />}
              {submit}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Availability, görünürlük ve erişim zamanı alanlarıdır. */
function AvailabilityFields({
  visible,
  from,
  until,
  onChange,
}: {
  visible: boolean
  from: string
  until: string
  onChange: (v: { visible?: boolean; from?: string; until?: string }) => void
}) {
  const { t } = useTranslation()
  return (
    <fieldset className="grid gap-3 rounded-xl border p-3 sm:grid-cols-3">
      <legend className="px-1 text-xs font-semibold text-muted-foreground">{t('lms.form.access')}</legend>
      <label className="flex items-center gap-2 text-sm sm:col-span-3">
        <Checkbox checked={visible} onCheckedChange={(v) => onChange({ visible: v === true })} />
        {t('lms.form.visible')}
      </label>
      <div className="space-y-1.5">
        <Label htmlFor="available-from">{t('lms.form.from')}</Label>
        <Input
          id="available-from"
          type="datetime-local"
          value={from}
          onChange={(e) => onChange({ from: e.target.value })}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="available-until">{t('lms.form.until')}</Label>
        <Input
          id="available-until"
          type="datetime-local"
          value={until}
          onChange={(e) => onChange({ until: e.target.value })}
        />
      </div>
    </fieldset>
  )
}

export type TopicValues = {
  title: string
  summary_html: string
  is_visible: boolean
  available_from: string | null
  available_until: string | null
}

/** TopicDialog, bölüm ekler ya da düzenler. */
export function TopicDialog({
  topic,
  onClose,
  onSave,
}: {
  topic?: Topic
  onClose: () => void
  onSave: (v: TopicValues) => Promise<string | null>
}) {
  const { t } = useTranslation()
  const [title, setTitle] = useState(topic?.title ?? '')
  const [summary, setSummary] = useState(topic?.summary_html ?? '')
  const [access, setAccess] = useState({
    visible: topic?.is_visible ?? true,
    from: toLocalInput(topic?.available_from),
    until: toLocalInput(topic?.available_until),
  })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  return (
    <EditorDialog
      title={topic ? t('lms.topic.editTitle') : t('lms.topic.addTitle')}
      icon={activityMeta.LABEL.icon}
      error={error}
      busy={busy}
      submit={t('common.save')}
      onClose={onClose}
      onSubmit={() => {
        if (!title.trim()) {
          setError(t('lms.form.titleRequired'))
          return
        }
        setBusy(true)
        void onSave({
          title: title.trim(),
          summary_html: summary,
          is_visible: access.visible,
          available_from: fromLocalInput(access.from),
          available_until: fromLocalInput(access.until),
        }).then((err) => {
          setError(err)
          setBusy(false)
        })
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="topic-title">{t('lms.form.title')}</Label>
        <Input
          id="topic-title"
          value={title}
          maxLength={200}
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t('lms.topic.titleHint')}
        />
      </div>
      <div className="space-y-1.5">
        <span className="text-sm font-medium">{t('lms.topic.summary')}</span>
        <RichTextEditor
          value={summary}
          onChange={setSummary}
          label={t('lms.topic.summary')}
          minHeight="min-h-24"
        />
      </div>
      <AvailabilityFields
        visible={access.visible}
        from={access.from}
        until={access.until}
        onChange={(v) => setAccess((a) => ({ ...a, ...v }))}
      />
    </EditorDialog>
  )
}

export type ActivityValues = {
  type: EditableType
  title: string
  description_html: string
  is_visible: boolean
  available_from: string | null
  available_until: string | null
  file_ids: string[]
  url?: string
  open_in_new_tab?: boolean
  body_html?: string
}

/**
 * ActivityDialog, bir etkinlik ekler ya da düzenler: türe göre dosya yükleme (dosya: tek,
 * klasör: çok), bağlantı, sayfa gövdesi ya da etiket metni.
 */
export function ActivityDialog({
  type,
  activity,
  body,
  onClose,
  onSave,
}: {
  type: EditableType
  activity?: Activity
  body?: string
  onClose: () => void
  onSave: (v: ActivityValues) => Promise<string | null>
}) {
  const { t } = useTranslation()
  const [title, setTitle] = useState(activity?.title ?? '')
  const [description, setDescription] = useState(activity?.description_html ?? '')
  const [files, setFiles] = useState<UploadedFile[]>(activity?.files ?? [])
  const [url, setUrl] = useState(activity?.url ?? 'https://')
  const [newTab, setNewTab] = useState(activity?.open_in_new_tab ?? true)
  const [page, setPage] = useState(body ?? '')
  const [access, setAccess] = useState({
    visible: activity?.is_visible ?? true,
    from: toLocalInput(activity?.available_from),
    until: toLocalInput(activity?.available_until),
  })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const typeName = t(`lms.types.${type}`)

  const submit = () => {
    if (!title.trim()) {
      setError(t('lms.form.titleRequired'))
      return
    }
    if ((type === 'RESOURCE' || type === 'FOLDER') && files.length === 0) {
      setError(t('lms.form.filesRequired'))
      return
    }
    setBusy(true)
    void onSave({
      type,
      title: title.trim(),
      description_html: description,
      is_visible: access.visible,
      available_from: fromLocalInput(access.from),
      available_until: fromLocalInput(access.until),
      file_ids: files.map((f) => f.id),
      ...(type === 'URL' ? { url: url.trim(), open_in_new_tab: newTab } : {}),
      ...(type === 'PAGE' ? { body_html: page } : {}),
    }).then((err) => {
      setError(err)
      setBusy(false)
    })
  }

  return (
    <EditorDialog
      title={
        activity
          ? t('lms.activity.editTitle', { type: typeName })
          : t('lms.activity.addTitle', { type: typeName })
      }
      icon={activityMeta[type].icon}
      error={error}
      busy={busy}
      submit={t('common.save')}
      onClose={onClose}
      onSubmit={submit}
    >
      <div className="space-y-1.5">
        <Label htmlFor="activity-title">{t('lms.form.title')}</Label>
        <Input id="activity-title" value={title} maxLength={200} onChange={(e) => setTitle(e.target.value)} />
      </div>
      {(type === 'RESOURCE' || type === 'FOLDER') && (
        <FileDropzone
          files={files}
          multiple={type === 'FOLDER'}
          onChange={(next) => {
            setFiles(next)
            // Başlık boşsa dosyanın adı önerilir (uzantısız).
            if (type === 'RESOURCE' && next[0]) {
              setTitle((v) => v || next[0]!.name.replace(/\.[^.]+$/, ''))
            }
          }}
        />
      )}
      {type === 'URL' && (
        <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
          <div className="space-y-1.5">
            <Label htmlFor="activity-url">{t('lms.form.url')}</Label>
            <Input
              id="activity-url"
              type="url"
              inputMode="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
          </div>
          <label className="flex items-center gap-2 pb-2.5 text-sm">
            <Checkbox checked={newTab} onCheckedChange={(v) => setNewTab(v === true)} />
            {t('lms.form.newTab')}
          </label>
        </div>
      )}
      {type === 'PAGE' && (
        <div className="space-y-1.5">
          <span className="text-sm font-medium">{t('lms.form.page')}</span>
          <RichTextEditor value={page} onChange={setPage} label={t('lms.form.page')} minHeight="min-h-56" />
        </div>
      )}
      <div className="space-y-1.5">
        <span className="text-sm font-medium">
          {type === 'LABEL' ? t('lms.form.labelText') : t('lms.form.description')}
        </span>
        <RichTextEditor
          value={description}
          onChange={setDescription}
          label={type === 'LABEL' ? t('lms.form.labelText') : t('lms.form.description')}
          minHeight="min-h-20"
        />
      </div>
      <AvailabilityFields
        visible={access.visible}
        from={access.from}
        until={access.until}
        onChange={(v) => setAccess((a) => ({ ...a, ...v }))}
      />
    </EditorDialog>
  )
}

/** SummaryDialog, ders alanının tanıtım metnini düzenler. */
export function SummaryDialog({
  value,
  onClose,
  onSave,
}: {
  value: string
  onClose: () => void
  onSave: (html: string) => Promise<string | null>
}) {
  const { t } = useTranslation()
  const [html, setHtml] = useState(value)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  return (
    <EditorDialog
      title={t('lms.space.summaryTitle')}
      description={t('lms.space.summaryHint')}
      icon={activityMeta.PAGE.icon}
      error={error}
      busy={busy}
      submit={t('common.save')}
      onClose={onClose}
      onSubmit={() => {
        setBusy(true)
        void onSave(html).then((err) => {
          setError(err)
          setBusy(false)
        })
      }}
    >
      <RichTextEditor value={html} onChange={setHtml} label={t('lms.space.summary')} minHeight="min-h-40" />
    </EditorDialog>
  )
}
