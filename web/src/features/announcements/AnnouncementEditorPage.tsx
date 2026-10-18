import { MegaphoneIcon, PlusIcon, SendIcon, XIcon } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import { ifMatch } from '@/shared/api/etag'
import {
  useCreateAnnouncementMutation,
  useGetAnnouncementQuery,
  useGetAnnouncementTargetsQuery,
  useUpdateAnnouncementMutation,
  type Announcement,
  type AnnouncementAudience,
  type AnnouncementInput,
  type AnnouncementTargets,
  type AnnouncementType,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { fromLocalInput, toLocalInput } from '@/shared/lib/datetime'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Card, CardContent } from '@/shared/ui/card'
import { Checkbox } from '@/shared/ui/checkbox'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { Spinner } from '@/shared/ui/spinner'
import { ErrorState } from '@/shared/ui/states'
import { Textarea } from '@/shared/ui/textarea'

import { announcementTypes } from './meta'

// Zengin metin editörü sadece duyuru yazılırken indirilir.
const RichTextEditor = lazy(() =>
  import('@/shared/ui/rich-text-editor').then((m) => ({ default: m.RichTextEditor })),
)

type Scope = AnnouncementAudience['scope_type']
type People = AnnouncementAudience['people']
type Row = { key: number; scope: Scope | ''; unit: string; people: People }

/** scopesOf, yetkinin hedefleyebildiği kapsamlardır. */
function scopesOf(targets: AnnouncementTargets): Scope[] {
  const out: Scope[] = []
  if (targets.university) {
    out.push('UNIVERSITY')
  }
  if (targets.faculties.length > 0) {
    out.push('FACULTY')
  }
  if (targets.departments.length > 0) {
    out.push('DEPARTMENT')
  }
  if (targets.programs.length > 0) {
    out.push('PROGRAM')
  }
  return out
}

function unitsOf(targets: AnnouncementTargets, scope: Scope | '') {
  return scope === 'FACULTY'
    ? targets.faculties
    : scope === 'DEPARTMENT'
      ? targets.departments
      : scope === 'PROGRAM'
        ? targets.programs
        : []
}

/** AnnouncementEditorPage, duyuru yazma ve düzenleme sayfasıdır (/duyuru-yonetimi/yeni, /:id/duzenle). */
export function AnnouncementEditorPage() {
  const { id } = useParams()
  const targets = useGetAnnouncementTargetsQuery()
  const existing = useGetAnnouncementQuery({ id: id ?? '' }, { skip: !id })
  if (targets.error || existing.error) {
    return (
      <ErrorState
        error={targets.error ?? existing.error}
        onRetry={() => void (targets.error ? targets.refetch() : existing.refetch())}
      />
    )
  }
  if (!targets.data || (id && !existing.data)) {
    return <Skeleton className="h-96" />
  }
  return <Editor targets={targets.data} announcement={existing.data} />
}

function Editor({ targets, announcement }: { targets: AnnouncementTargets; announcement?: Announcement }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const errorMessage = useErrorMessage()
  const [create, creating] = useCreateAnnouncementMutation()
  const [update, updating] = useUpdateAnnouncementMutation()
  const scopes = scopesOf(targets)
  const [title, setTitle] = useState(announcement?.title ?? '')
  const [type, setType] = useState<AnnouncementType>(announcement?.type ?? 'GENERAL')
  const [summary, setSummary] = useState(announcement?.summary ?? '')
  const [body, setBody] = useState(announcement?.body_html ?? '')
  const [from, setFrom] = useState(toLocalInput(announcement?.publish_from))
  const [until, setUntil] = useState(toLocalInput(announcement?.publish_until))
  const [pinned, setPinned] = useState(announcement?.is_pinned ?? false)
  const [rows, setRows] = useState<Row[]>(() =>
    announcement
      ? announcement.audiences.map((a, i) => ({
          key: i,
          scope: a.scope_type,
          unit: a.scope_id ?? '',
          people: a.people,
        }))
      : [
          {
            key: 0,
            scope: scopes.includes('DEPARTMENT') ? 'DEPARTMENT' : (scopes[0] ?? ''),
            unit: '',
            people: 'ALL',
          },
        ],
  )
  const [error, setError] = useState<string | null>(null)
  const busy = creating.isLoading || updating.isLoading

  const setRow = (key: number, patch: Partial<Row>) =>
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const submit = async (publish: boolean) => {
    if (!title.trim()) {
      setError(t('announcements.editor.titleRequired'))
      return
    }
    if (rows.length === 0 || rows.some((r) => !r.scope || (r.scope !== 'UNIVERSITY' && !r.unit))) {
      setError(t('announcements.editor.audienceRequired'))
      return
    }
    setError(null)
    const body_: AnnouncementInput = {
      title: title.trim(),
      type,
      summary: summary.trim(),
      body_html: body,
      publish_from: fromLocalInput(from),
      publish_until: fromLocalInput(until),
      is_pinned: pinned,
      audiences: rows.map((r) => ({
        scope_type: r.scope as Scope,
        scope_id: r.scope === 'UNIVERSITY' ? null : r.unit,
        people: r.scope === 'PROGRAM' ? 'STUDENTS' : r.people,
      })),
    }
    const res = announcement
      ? await update({
          id: announcement.id,
          'If-Match': ifMatch(announcement.version),
          announcementInput: body_,
        })
      : await create({ announcementInput: { ...body_, publish } })
    if (res.error) {
      setError(errorMessage(res.error))
      return
    }
    toast.success(
      announcement
        ? t('announcements.editor.saved')
        : publish
          ? t('announcements.actions.published')
          : t('announcements.editor.draftSaved'),
    )
    void navigate(`/duyurular/${res.data.id}`)
  }

  return (
    <>
      <PageHeader
        icon={MegaphoneIcon}
        tone="orange"
        title={announcement ? t('announcements.editor.editTitle') : t('announcements.editor.newTitle')}
        description={t('announcements.editor.description')}
      />
      <Card>
        <CardContent className="space-y-5 pt-6">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_200px]">
            <div className="space-y-2">
              <Label htmlFor="ann-title">{t('announcements.editor.title')}</Label>
              <Input
                id="ann-title"
                value={title}
                maxLength={200}
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="ann-type">{t('announcements.editor.type')}</Label>
              <Select value={type} onValueChange={(v) => setType(v as AnnouncementType)}>
                <SelectTrigger id="ann-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {announcementTypes.map((x) => (
                    <SelectItem key={x} value={x}>
                      {t(`announcements.types.${x}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="ann-summary">{t('announcements.editor.summary')}</Label>
            <Textarea
              id="ann-summary"
              value={summary}
              maxLength={500}
              rows={2}
              onChange={(e) => setSummary(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t('announcements.editor.summaryHint')}</p>
          </div>
          <div className="space-y-2">
            <span className="text-sm font-medium">{t('announcements.editor.body')}</span>
            <Suspense fallback={<Skeleton className="h-48" />}>
              <RichTextEditor
                value={body}
                onChange={setBody}
                label={t('announcements.editor.body')}
                minHeight="min-h-48"
              />
            </Suspense>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="ann-from">{t('announcements.editor.from')}</Label>
              <Input
                id="ann-from"
                type="datetime-local"
                value={from}
                onChange={(e) => setFrom(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">{t('announcements.editor.fromHint')}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="ann-until">{t('announcements.editor.until')}</Label>
              <Input
                id="ann-until"
                type="datetime-local"
                value={until}
                onChange={(e) => setUntil(e.target.value)}
              />
            </div>
            <label className="flex items-center gap-2 self-center text-sm">
              <Checkbox checked={pinned} onCheckedChange={(v) => setPinned(v === true)} />
              {t('announcements.editor.pinned')}
            </label>
          </div>

          <fieldset className="space-y-3">
            <legend className="text-sm font-bold">{t('announcements.audience')}</legend>
            {rows.map((r, i) => {
              const units = unitsOf(targets, r.scope)
              const n = i + 1
              return (
                <div
                  key={r.key}
                  className="grid gap-2 rounded-xl border border-border/70 p-3 sm:grid-cols-[160px_minmax(0,1fr)_150px_auto]"
                >
                  <Select
                    value={r.scope}
                    onValueChange={(v) => setRow(r.key, { scope: v as Scope, unit: '' })}
                  >
                    <SelectTrigger className="w-full" aria-label={t('announcements.editor.scopeFor', { n })}>
                      <SelectValue placeholder={t('announcements.editor.scope')} />
                    </SelectTrigger>
                    <SelectContent>
                      {scopes.map((s) => (
                        <SelectItem key={s} value={s}>
                          {t(`announcements.scopes.${s}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {r.scope === 'UNIVERSITY' ? (
                    <span className="self-center text-sm text-muted-foreground">
                      {t('announcements.university')}
                    </span>
                  ) : (
                    <Select
                      value={r.unit}
                      onValueChange={(v) => setRow(r.key, { unit: v })}
                      disabled={!r.scope}
                    >
                      <SelectTrigger className="w-full" aria-label={t('announcements.editor.unitFor', { n })}>
                        <SelectValue placeholder={t('announcements.editor.unit')} />
                      </SelectTrigger>
                      <SelectContent>
                        {units.map((u) => (
                          <SelectItem key={u.id} value={u.id}>
                            {u.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                  <Select
                    value={r.scope === 'PROGRAM' ? 'STUDENTS' : r.people}
                    onValueChange={(v) => setRow(r.key, { people: v as People })}
                    disabled={r.scope === 'PROGRAM'}
                  >
                    <SelectTrigger className="w-full" aria-label={t('announcements.editor.peopleFor', { n })}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {(['ALL', 'STUDENTS', 'STAFF'] as const).map((p) => (
                        <SelectItem key={p} value={p}>
                          {t(`announcements.people.${p}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('announcements.editor.removeAudience', { n })}
                    disabled={rows.length === 1}
                    onClick={() => setRows((rs) => rs.filter((x) => x.key !== r.key))}
                  >
                    <XIcon />
                  </Button>
                </div>
              )
            })}
            <Button
              variant="soft"
              size="sm"
              onClick={() =>
                setRows((rs) => [
                  ...rs,
                  {
                    key: Math.max(...rs.map((x) => x.key), 0) + 1,
                    scope: scopes[0] ?? '',
                    unit: '',
                    people: 'ALL',
                  },
                ])
              }
            >
              <PlusIcon />
              {t('announcements.editor.addAudience')}
            </Button>
          </fieldset>

          <div className="flex flex-wrap justify-end gap-2 border-t border-border/60 pt-4">
            <Button variant="outline" onClick={() => void navigate(-1)}>
              {t('common.cancel')}
            </Button>
            {announcement ? (
              <Button disabled={busy} onClick={() => void submit(false)}>
                {busy && <Spinner />}
                {t('announcements.editor.save')}
              </Button>
            ) : (
              <>
                <Button variant="outline" disabled={busy} onClick={() => void submit(false)}>
                  {t('announcements.editor.saveDraft')}
                </Button>
                <Button disabled={busy} onClick={() => void submit(true)}>
                  {busy ? <Spinner /> : <SendIcon />}
                  {t('announcements.actions.publish')}
                </Button>
              </>
            )}
          </div>
        </CardContent>
      </Card>
    </>
  )
}
