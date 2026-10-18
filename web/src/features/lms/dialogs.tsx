import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import type { ParseKeys } from 'i18next'

import type {
  Activity,
  AssignmentInput,
  AssignmentSettings,
  LatePolicy,
  SubmissionType,
  Topic,
} from '@/shared/api/generated'
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Spinner } from '@/shared/ui/spinner'

import { FileDropzone, type UploadedFile } from './FileDropzone'
import { activityMeta, fromLocalInput, toLocalInput, type EditableType } from './meta'

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
  assignment?: AssignmentInput
  forum?: { students_can_start_threads: boolean }
}

type AssignmentForm = {
  opens: string
  due: string
  cutoff: string
  file: boolean
  text: boolean
  maxFiles: string
  maxSizeMb: string
  extensions: string
  attempts: string
  maxScore: string
  policy: LatePolicy
  penalty: string
}

/** nextWeek, bir hafta sonrasının 23:59'udur (yeni ödevin varsayılan son tarihi). */
function nextWeek(): string {
  const d = new Date()
  d.setDate(d.getDate() + 7)
  d.setHours(23, 59, 0, 0)
  return toLocalInput(d.toISOString())
}

function assignmentForm(a?: AssignmentSettings | null): AssignmentForm {
  return {
    opens: toLocalInput(a?.opens_at),
    due: a ? toLocalInput(a.due_at) : nextWeek(),
    cutoff: toLocalInput(a?.cutoff_at),
    file: a ? a.submission_types.includes('FILE') : true,
    text: a ? a.submission_types.includes('ONLINE_TEXT') : false,
    maxFiles: String(a?.max_files ?? 5),
    maxSizeMb: a ? String(Math.round((a.max_file_size_bytes / 1048576) * 10) / 10) : '',
    extensions: (a?.allowed_extensions ?? []).map((e) => e.replace(/^\./, '')).join(', '),
    attempts: String(a?.max_attempts ?? 1),
    maxScore: String(a?.max_score ?? 100),
    policy: a?.late_policy ?? 'ALLOW',
    penalty: String(a?.late_penalty_pct_per_day ?? 10),
  }
}

const num = (v: string) => Number(v.replace(',', '.'))

/** assignmentInput, formu istek gövdesine çevirir; hatalıysa hata anahtarını döndürür. */
function assignmentInput(f: AssignmentForm): AssignmentInput | ParseKeys {
  const types: SubmissionType[] = [
    ...(f.file ? (['FILE'] as const) : []),
    ...(f.text ? (['ONLINE_TEXT'] as const) : []),
  ]
  if (types.length === 0) {
    return 'lms.assignment.form.typesRequired'
  }
  if (f.policy !== 'ALLOW' && !f.due) {
    return 'lms.assignment.form.dueRequired'
  }
  const score = num(f.maxScore)
  if (!(score > 0 && score <= 1000)) {
    return 'lms.assignment.form.scoreInvalid'
  }
  return {
    opens_at: fromLocalInput(f.opens),
    due_at: fromLocalInput(f.due),
    cutoff_at: f.policy === 'DENY' ? null : fromLocalInput(f.cutoff),
    submission_types: types,
    max_files: num(f.maxFiles) || 1,
    ...(f.maxSizeMb.trim() ? { max_file_size_bytes: Math.round(num(f.maxSizeMb) * 1048576) } : {}),
    allowed_extensions: f.extensions
      .split(/[\s,;]+/)
      .map((e) => e.trim())
      .filter(Boolean),
    max_attempts: num(f.attempts) || 1,
    max_score: score,
    late_policy: f.policy,
    late_penalty_pct_per_day: f.policy === 'PENALTY' ? num(f.penalty) : 0,
  }
}

/** AssignmentFields, ödevin teslim ayarlarıdır: tarihler, teslim türleri ve sınırlar, puan, geç teslim. */
function AssignmentFields({
  value,
  onChange,
}: {
  value: AssignmentForm
  onChange: (v: Partial<AssignmentForm>) => void
}) {
  const { t } = useTranslation()
  return (
    <fieldset className="space-y-3 rounded-xl border border-rose-200/70 bg-rose-50/30 p-3 dark:border-rose-500/20 dark:bg-rose-500/5">
      <legend className="px-1 text-xs font-semibold text-muted-foreground">
        {t('lms.assignment.form.legend')}
      </legend>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-1.5">
          <Label htmlFor="asg-opens">{t('lms.assignment.form.opens')}</Label>
          <Input
            id="asg-opens"
            type="datetime-local"
            value={value.opens}
            onChange={(e) => onChange({ opens: e.target.value })}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="asg-due">{t('lms.assignment.form.due')}</Label>
          <Input
            id="asg-due"
            type="datetime-local"
            value={value.due}
            onChange={(e) => onChange({ due: e.target.value })}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="asg-cutoff">{t('lms.assignment.form.cutoff')}</Label>
          <Input
            id="asg-cutoff"
            type="datetime-local"
            value={value.policy === 'DENY' ? '' : value.cutoff}
            disabled={value.policy === 'DENY'}
            onChange={(e) => onChange({ cutoff: e.target.value })}
          />
        </div>
      </div>
      <div className="flex flex-wrap gap-x-5 gap-y-2">
        <span className="text-sm font-medium">{t('lms.assignment.form.types')}</span>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={value.file} onCheckedChange={(v) => onChange({ file: v === true })} />
          {t('lms.assignment.types.FILE')}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={value.text} onCheckedChange={(v) => onChange({ text: v === true })} />
          {t('lms.assignment.types.ONLINE_TEXT')}
        </label>
      </div>
      {value.file && (
        <div className="grid gap-3 sm:grid-cols-[110px_130px_1fr]">
          <div className="space-y-1.5">
            <Label htmlFor="asg-files">{t('lms.assignment.form.maxFiles')}</Label>
            <Input
              id="asg-files"
              type="number"
              min={1}
              max={20}
              value={value.maxFiles}
              onChange={(e) => onChange({ maxFiles: e.target.value })}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asg-size">{t('lms.assignment.form.maxSize')}</Label>
            <Input
              id="asg-size"
              inputMode="decimal"
              placeholder={t('lms.assignment.form.maxSizeDefault')}
              value={value.maxSizeMb}
              onChange={(e) => onChange({ maxSizeMb: e.target.value })}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asg-ext">{t('lms.assignment.form.extensions')}</Label>
            <Input
              id="asg-ext"
              placeholder={t('lms.assignment.form.extensionsHint')}
              value={value.extensions}
              onChange={(e) => onChange({ extensions: e.target.value })}
            />
          </div>
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-[110px_110px_1fr_140px]">
        <div className="space-y-1.5">
          <Label htmlFor="asg-attempts">{t('lms.assignment.form.attempts')}</Label>
          <Input
            id="asg-attempts"
            type="number"
            min={1}
            max={10}
            value={value.attempts}
            onChange={(e) => onChange({ attempts: e.target.value })}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="asg-score">{t('lms.assignment.form.maxScore')}</Label>
          <Input
            id="asg-score"
            inputMode="decimal"
            value={value.maxScore}
            onChange={(e) => onChange({ maxScore: e.target.value })}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="asg-policy">{t('lms.assignment.form.policy')}</Label>
          <Select value={value.policy} onValueChange={(v) => onChange({ policy: v as LatePolicy })}>
            <SelectTrigger id="asg-policy" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['ALLOW', 'PENALTY', 'DENY'] as const).map((p) => (
                <SelectItem key={p} value={p}>
                  {t(`lms.assignment.policy.${p}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {value.policy === 'PENALTY' && (
          <div className="space-y-1.5">
            <Label htmlFor="asg-penalty">{t('lms.assignment.form.penalty')}</Label>
            <Input
              id="asg-penalty"
              inputMode="decimal"
              value={value.penalty}
              onChange={(e) => onChange({ penalty: e.target.value })}
            />
          </div>
        )}
      </div>
    </fieldset>
  )
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
  const [asg, setAsg] = useState(() => assignmentForm(activity?.assignment))
  const [studentsStart, setStudentsStart] = useState(activity?.forum?.students_can_start_threads ?? true)
  const announcements = activity?.forum?.kind === 'ANNOUNCEMENTS'
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const typeName = t(`lms.types.${type}`)
  const descriptionLabel =
    type === 'LABEL'
      ? t('lms.form.labelText')
      : type === 'ASSIGNMENT'
        ? t('lms.assignment.form.instructions')
        : type === 'FORUM'
          ? t('lms.forum.description')
          : t('lms.form.description')

  const submit = () => {
    if (!title.trim()) {
      setError(t('lms.form.titleRequired'))
      return
    }
    if ((type === 'RESOURCE' || type === 'FOLDER') && files.length === 0) {
      setError(t('lms.form.filesRequired'))
      return
    }
    let assignment: AssignmentInput | undefined
    if (type === 'ASSIGNMENT') {
      const res = assignmentInput(asg)
      if (typeof res === 'string') {
        setError(t(res))
        return
      }
      assignment = res
    }
    setBusy(true)
    void onSave({
      assignment,
      ...(type === 'FORUM' ? { forum: { students_can_start_threads: !announcements && studentsStart } } : {}),
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
      {type === 'FORUM' && !announcements && (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={studentsStart} onCheckedChange={(v) => setStudentsStart(v === true)} />
          {t('lms.forum.studentsCanStart')}
        </label>
      )}
      {type === 'ASSIGNMENT' && (
        <AssignmentFields value={asg} onChange={(v) => setAsg((a) => ({ ...a, ...v }))} />
      )}
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
        <span className="text-sm font-medium">{descriptionLabel}</span>
        <RichTextEditor
          value={description}
          onChange={setDescription}
          label={descriptionLabel}
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
