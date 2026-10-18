import {
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowUpIcon,
  DownloadIcon,
  EllipsisVerticalIcon,
  EyeIcon,
  EyeOffIcon,
  ExternalLinkIcon,
  InfoIcon,
  LockIcon,
  MailIcon,
  MonitorPlayIcon,
  PencilIcon,
  PencilRulerIcon,
  PlusIcon,
  Trash2Icon,
  UsersRoundIcon,
} from 'lucide-react'
import { lazy, Suspense, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { navTheme } from '@/features/shell/nav'
import { formatDateTime, formatRelative } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  agoraApi,
  useCreateActivityMutation,
  useCreateTopicMutation,
  useDeleteActivityMutation,
  useDeleteTopicMutation,
  useGetSpaceQuery,
  useSetSpaceLayoutMutation,
  useSetSpaceSummaryMutation,
  useUpdateActivityMutation,
  useUpdateTopicMutation,
  type Activity,
  type Space,
  type Topic,
} from '@/shared/api/generated'
import { formatBytes } from '@/shared/api/upload'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { RichText } from '@/shared/ui/rich-text'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import type { ActivityValues, TopicValues } from './dialogs'
import { AvailabilityBadges } from './AvailabilityBadges'
import { useDownload } from './download'
import { activityMeta, editableTypes, hostOf, inputOf, type EditableType } from './meta'

// Düzenleme pencereleri (zengin metin düzenleyicisiyle) sadece düzenleyene indirilir.
const SummaryDialog = lazy(() => import('./dialogs').then((m) => ({ default: m.SummaryDialog })))
const TopicDialog = lazy(() => import('./dialogs').then((m) => ({ default: m.TopicDialog })))
const ActivityDialog = lazy(() => import('./dialogs').then((m) => ({ default: m.ActivityDialog })))

type Editing =
  | { kind: 'summary' }
  | { kind: 'topic'; topic?: Topic }
  | { kind: 'activity'; topicId: string; type: EditableType; activity?: Activity; body?: string }
  | { kind: 'delete-topic'; topic: Topic }
  | { kind: 'delete-activity'; activity: Activity }

/**
 * SpacePage, bir dersin alanıdır: bölümler ve etkinlikler, ders bilgisi ve öğretim
 * elemanları. Öğretim elemanı düzenleme modunda bölüm ve etkinlik ekler, düzenler, gizler,
 * sıralar ve siler; öğrenci gizlileri görmez, açılmamışların sadece adını ve tarihini görür.
 */
export function SpacePage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const localized = useLocalized()
  const message = useErrorMessage()
  const dispatch = useAppDispatch()
  const { data: space, error, refetch } = useGetSpaceQuery({ id })
  const [editMode, setEditMode] = useState(false)
  const [editing, setEditing] = useState<Editing | null>(null)
  const [setSummary] = useSetSpaceSummaryMutation()
  const [createTopic] = useCreateTopicMutation()
  const [updateTopic] = useUpdateTopicMutation()
  const [deleteTopic] = useDeleteTopicMutation()
  const [createActivity] = useCreateActivityMutation()
  const [updateActivity] = useUpdateActivityMutation()
  const [deleteActivity] = useDeleteActivityMutation()
  const [setLayout] = useSetSpaceLayoutMutation()

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!space) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-12 w-96" />
        <Skeleton className="h-64" />
      </div>
    )
  }

  const editable = space.can_edit && editMode
  const fail = (res: { error?: unknown }) => (res.error ? message(res.error) : null)

  // Sıra: bütün bölümler ve etkinlikler; taşınan öğe yer değiştirir.
  const layoutOf = (topics: Topic[]) =>
    topics.map((tp) => ({ id: tp.id, activity_ids: tp.activities.map((a) => a.id) }))
  const saveLayout = async (topics: Topic[]) => {
    const res = await setLayout({ id, body: { topics: layoutOf(topics) } })
    if ('error' in res && res.error) {
      toast.error(message(res.error))
    }
  }
  const moveTopic = (i: number, d: -1 | 1) => {
    const topics = [...space.topics]
    const [moved] = topics.splice(i, 1)
    topics.splice(i + d, 0, moved!)
    void saveLayout(topics)
  }
  const moveActivity = (ti: number, ai: number, d: -1 | 1) => {
    const topics = space.topics.map((tp) => ({ ...tp, activities: [...tp.activities] }))
    const list = topics[ti]!.activities
    const [moved] = list.splice(ai, 1)
    const target = ai + d
    if (target >= 0 && target <= list.length) {
      list.splice(target, 0, moved!)
    } else if (d < 0 && ti > 0) {
      topics[ti - 1]!.activities.push(moved!) // önceki bölümün sonuna
    } else if (d > 0 && ti < topics.length - 1) {
      topics[ti + 1]!.activities.unshift(moved!) // sonraki bölümün başına
    } else {
      return
    }
    void saveLayout(topics)
  }

  const toggleActivity = async (a: Activity) => {
    let body: string | undefined
    if (a.type === 'PAGE') {
      body =
        (
          await dispatch(
            agoraApi.endpoints.getActivity.initiate({ id: a.id }, { subscribe: false, forceRefetch: true }),
          )
        ).data?.body_html ?? ''
    }
    const res = await updateActivity({
      id: a.id,
      'If-Match': ifMatch(a.version),
      activityInput: { ...inputOf(a, body), is_visible: !a.is_visible },
    })
    if ('error' in res && res.error) {
      toast.error(message(res.error))
    }
  }

  const editActivity = async (topicId: string, a: Activity) => {
    let body: string | undefined
    if (a.type === 'PAGE') {
      const res = await dispatch(
        agoraApi.endpoints.getActivity.initiate({ id: a.id }, { subscribe: false, forceRefetch: true }),
      )
      body = res.data?.body_html ?? ''
    }
    setEditing({ kind: 'activity', topicId, type: a.type, activity: a, body })
  }

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
        <Link to="/derslerim">
          <ArrowLeftIcon />
          {t('lms.mySpaces.title')}
        </Link>
      </Button>
      <PageHeader
        {...navTheme('/derslerim')}
        title={localized(space.course)}
        description={[
          `${space.course.code}-${space.section_code} · ${space.term.code}`,
          space.role !== 'STUDENT' ? t(`lms.roles.${space.role}`) : null,
        ]
          .filter(Boolean)
          .join(' · ')}
        actions={
          space.can_edit && (
            <Button
              variant={editMode ? 'default' : 'outline'}
              aria-pressed={editMode}
              onClick={() => setEditMode((v) => !v)}
            >
              <PencilRulerIcon />
              {editMode ? t('lms.space.editing') : t('lms.space.edit')}
            </Button>
          )
        }
      />

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
        <div className="space-y-4">
          {space.topics.length === 0 && (
            <EmptyState icon={MonitorPlayIcon} tone="indigo">
              {space.can_edit ? t('lms.space.emptyEditor') : t('lms.space.empty')}
            </EmptyState>
          )}
          {space.topics.map((topic, ti) => (
            <TopicSection
              key={topic.id}
              topic={topic}
              space={space}
              editable={editable}
              first={ti === 0}
              last={ti === space.topics.length - 1}
              onMove={(d) => moveTopic(ti, d)}
              onMoveActivity={(ai, d) => moveActivity(ti, ai, d)}
              onEdit={() => setEditing({ kind: 'topic', topic })}
              onDelete={() => setEditing({ kind: 'delete-topic', topic })}
              onAdd={(type) => setEditing({ kind: 'activity', topicId: topic.id, type })}
              onEditActivity={(a) => void editActivity(topic.id, a)}
              onToggleActivity={(a) => void toggleActivity(a)}
              onDeleteActivity={(a) => setEditing({ kind: 'delete-activity', activity: a })}
            />
          ))}
          {editable && (
            <Button
              variant="outline"
              className="w-full border-dashed"
              onClick={() => setEditing({ kind: 'topic' })}
            >
              <PlusIcon />
              {t('lms.topic.add')}
            </Button>
          )}
        </div>

        <aside className="space-y-4">
          <Card>
            <CardHeader className="flex items-center gap-3">
              <IconChip icon={InfoIcon} tone="sky" />
              <CardTitle className="flex-1">{t('lms.space.info')}</CardTitle>
              {editable && (
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t('lms.space.summaryTitle')}
                  onClick={() => setEditing({ kind: 'summary' })}
                >
                  <PencilIcon />
                </Button>
              )}
            </CardHeader>
            <CardContent>
              {space.summary_html ? (
                <RichText html={space.summary_html} />
              ) : (
                <p className="text-sm text-muted-foreground">{t('lms.space.noSummary')}</p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="flex items-center gap-3">
              <IconChip icon={UsersRoundIcon} tone="violet" />
              <CardTitle>{t('lms.space.instructors')}</CardTitle>
            </CardHeader>
            <CardContent>
              <ul className="space-y-3">
                {space.instructors.map((p) => (
                  <li key={p.user_id} className="flex items-center gap-3">
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-violet-500 to-indigo-600 text-xs font-bold text-white">
                      {p.first_name[0]}
                      {p.last_name[0]}
                    </span>
                    <span className="min-w-0 flex-1 text-sm">
                      <span className="block truncate font-semibold">
                        {[p.title, p.first_name, p.last_name].filter(Boolean).join(' ')}
                      </span>
                      <span className="text-xs text-muted-foreground">{t(`lms.roles.${p.role}`)}</span>
                    </span>
                    <a
                      href={`mailto:${p.email}`}
                      aria-label={t('lms.space.email', { name: `${p.first_name} ${p.last_name}` })}
                      className="rounded-lg p-2 text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                      <MailIcon className="size-4" />
                    </a>
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        </aside>
      </div>

      <Suspense fallback={null}>
        {editing?.kind === 'summary' && (
          <SummaryDialog
            value={space.summary_html}
            onClose={() => setEditing(null)}
            onSave={async (html) => {
              const err = fail(await setSummary({ id, body: { summary_html: html } }))
              if (!err) {
                setEditing(null)
              }
              return err
            }}
          />
        )}
        {editing?.kind === 'topic' && (
          <TopicDialog
            topic={editing.topic}
            onClose={() => setEditing(null)}
            onSave={async (v: TopicValues) => {
              const res = editing.topic
                ? await updateTopic({
                    id: editing.topic.id,
                    'If-Match': ifMatch(editing.topic.version),
                    topicInput: v,
                  })
                : await createTopic({ id, topicInput: v })
              const err = fail(res)
              if (!err) {
                toast.success(editing.topic ? t('lms.topic.saved') : t('lms.topic.added'))
                setEditing(null)
              }
              return err
            }}
          />
        )}
        {editing?.kind === 'activity' && (
          <ActivityDialog
            type={editing.type}
            activity={editing.activity}
            body={editing.body}
            onClose={() => setEditing(null)}
            onSave={async (v: ActivityValues) => {
              const res = editing.activity
                ? await updateActivity({
                    id: editing.activity.id,
                    'If-Match': ifMatch(editing.activity.version),
                    activityInput: v,
                  })
                : await createActivity({ id: editing.topicId, activityInput: v })
              const err = fail(res)
              if (!err) {
                toast.success(editing.activity ? t('lms.activity.saved') : t('lms.activity.added'))
                setEditing(null)
              }
              return err
            }}
          />
        )}
      </Suspense>
      {editing?.kind === 'delete-topic' && (
        <ConfirmDialog
          title={t('lms.topic.deleteTitle', { title: editing.topic.title })}
          description={t('lms.topic.deleteDescription')}
          confirm={t('lms.delete')}
          destructive
          onClose={() => setEditing(null)}
          onConfirm={async () => {
            const res = await deleteTopic({ id: editing.topic.id })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return
            }
            setEditing(null)
          }}
        />
      )}
      {editing?.kind === 'delete-activity' && (
        <ConfirmDialog
          title={t('lms.activity.deleteTitle', { title: editing.activity.title })}
          description={t('lms.activity.deleteDescription')}
          confirm={t('lms.delete')}
          destructive
          onClose={() => setEditing(null)}
          onConfirm={async () => {
            const res = await deleteActivity({ id: editing.activity.id })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return
            }
            setEditing(null)
          }}
        />
      )}
    </>
  )
}

/** ItemMenu, bölüm ya da etkinliğin düzenleme menüsüdür (düzenle, gizle, sil). */
function ItemMenu({ label, children }: { label: string; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="icon" variant="ghost" className="size-8" aria-label={t('lms.actions', { name: label })}>
          <EllipsisVerticalIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  )
}

function MoveButtons({
  first,
  last,
  onMove,
  label,
}: {
  first: boolean
  last: boolean
  onMove: (d: -1 | 1) => void
  label: string
}) {
  const { t } = useTranslation()
  return (
    <span className="inline-flex">
      <Button
        size="icon"
        variant="ghost"
        className="size-8"
        disabled={first}
        aria-label={t('lms.moveUp', { name: label })}
        onClick={() => onMove(-1)}
      >
        <ArrowUpIcon />
      </Button>
      <Button
        size="icon"
        variant="ghost"
        className="size-8"
        disabled={last}
        aria-label={t('lms.moveDown', { name: label })}
        onClick={() => onMove(1)}
      >
        <ArrowDownIcon />
      </Button>
    </span>
  )
}

function TopicSection({
  topic,
  space,
  editable,
  first,
  last,
  onMove,
  onMoveActivity,
  onEdit,
  onDelete,
  onAdd,
  onEditActivity,
  onToggleActivity,
  onDeleteActivity,
}: {
  topic: Topic
  space: Space
  editable: boolean
  first: boolean
  last: boolean
  onMove: (d: -1 | 1) => void
  onMoveActivity: (index: number, d: -1 | 1) => void
  onEdit: () => void
  onDelete: () => void
  onAdd: (type: EditableType) => void
  onEditActivity: (a: Activity) => void
  onToggleActivity: (a: Activity) => void
  onDeleteActivity: (a: Activity) => void
}) {
  const { t } = useTranslation()
  const locked = topic.availability !== 'OPEN' && !space.can_edit
  return (
    <section
      aria-labelledby={`topic-${topic.id}`}
      className={cn(
        'overflow-hidden rounded-2xl border border-border/70 bg-card shadow-xs',
        !topic.is_visible && 'border-dashed opacity-80',
      )}
    >
      <header className="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-4 py-3">
        <h2 id={`topic-${topic.id}`} className="min-w-0 flex-1 text-base font-bold">
          {topic.title}
        </h2>
        <AvailabilityBadges item={topic} />
        {editable && (
          <span className="flex items-center gap-0.5">
            <MoveButtons first={first} last={last} onMove={onMove} label={topic.title} />
            <ItemMenu label={topic.title}>
              <DropdownMenuItem onSelect={onEdit}>
                <PencilIcon />
                {t('lms.edit')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={onDelete}>
                <Trash2Icon />
                {t('lms.delete')}
              </DropdownMenuItem>
            </ItemMenu>
          </span>
        )}
      </header>
      <div className="space-y-2 p-4">
        {locked ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <LockIcon className="size-4" aria-hidden />
            {topic.availability === 'NOT_YET' ? t('lms.topicNotYet') : t('lms.topicClosed')}
          </p>
        ) : (
          <>
            <RichText html={topic.summary_html} className="text-muted-foreground" />
            {topic.activities.length === 0 && !editable && (
              <p className="text-sm text-muted-foreground">{t('lms.topicEmpty')}</p>
            )}
            <ul className="space-y-1.5">
              {topic.activities.map((a, ai) => (
                <ActivityRow
                  key={a.id}
                  activity={a}
                  sectionId={space.section_id}
                  editable={editable}
                  first={first && ai === 0}
                  last={last && ai === topic.activities.length - 1}
                  onMove={(d) => onMoveActivity(ai, d)}
                  onEdit={() => onEditActivity(a)}
                  onToggle={() => onToggleActivity(a)}
                  onDelete={() => onDeleteActivity(a)}
                />
              ))}
            </ul>
            {editable && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button size="sm" variant="soft" className="mt-1">
                    <PlusIcon />
                    {t('lms.activity.add')}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  {editableTypes.map((type) => {
                    const Icon = activityMeta[type].icon
                    return (
                      <DropdownMenuItem key={type} onSelect={() => onAdd(type)}>
                        <Icon />
                        {t(`lms.types.${type}`)}
                      </DropdownMenuItem>
                    )
                  })}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </>
        )}
      </div>
    </section>
  )
}

function ActivityRow({
  activity: a,
  sectionId,
  editable,
  first,
  last,
  onMove,
  onEdit,
  onToggle,
  onDelete,
}: {
  activity: Activity
  sectionId: string
  editable: boolean
  first: boolean
  last: boolean
  onMove: (d: -1 | 1) => void
  onEdit: () => void
  onToggle: () => void
  onDelete: () => void
}) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const download = useDownload()
  const locale = i18n.language.startsWith('en') ? 'en-GB' : 'tr-TR'
  const meta = activityMeta[a.type]
  const open = a.availability === 'OPEN' || editable
  const controls = editable && (
    <span className="flex shrink-0 items-center gap-0.5">
      <MoveButtons first={first} last={last} onMove={onMove} label={a.title} />
      <ItemMenu label={a.title}>
        {(editableTypes as readonly string[]).includes(a.type) && (
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon />
            {t('lms.edit')}
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onSelect={onToggle}>
          {a.is_visible ? <EyeOffIcon /> : <EyeIcon />}
          {a.is_visible ? t('lms.activity.hide') : t('lms.activity.show')}
        </DropdownMenuItem>
        {/* Duyurular forumu silinmez, gizlenebilir. */}
        {a.forum?.kind !== 'ANNOUNCEMENTS' && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={onDelete}>
              <Trash2Icon />
              {t('lms.delete')}
            </DropdownMenuItem>
          </>
        )}
      </ItemMenu>
    </span>
  )

  if (a.type === 'LABEL') {
    return (
      <li className={cn('flex items-start gap-2 rounded-xl px-1 py-1', !a.is_visible && 'opacity-60')}>
        <div className="min-w-0 flex-1">
          {editable && <p className="mb-1 text-xs font-semibold text-muted-foreground">{a.title}</p>}
          <RichText html={a.description_html} />
        </div>
        <AvailabilityBadges item={a} />
        {controls}
      </li>
    )
  }

  const file = a.files[0]
  const detail =
    a.type === 'RESOURCE' && file
      ? `${file.name} · ${formatBytes(file.size, locale)}`
      : a.type === 'FOLDER'
        ? t('lms.files', { count: a.files.length })
        : a.type === 'URL'
          ? hostOf(a.url)
          : a.type === 'FORUM' && a.forum
            ? a.forum.threads > 0 && a.forum.last_post_at
              ? `${t('lms.forum.threads', { count: a.forum.threads })} · ${t('lms.forum.lastPost', { time: formatRelative(a.forum.last_post_at) })}`
              : t('lms.forum.noThreadsShort')
            : a.type === 'ASSIGNMENT' && a.assignment
              ? a.assignment.due_at
                ? t('lms.assignment.dueShort', { time: formatDateTime(a.assignment.due_at) })
                : t('lms.assignment.noDue')
              : t(`lms.types.${a.type}`)

  const activate = () => {
    if (!open) {
      return
    }
    if (a.type === 'RESOURCE' && file) {
      void download(a.id, file.id)
      return
    }
    if (a.type === 'URL' && a.url) {
      if (a.open_in_new_tab) {
        window.open(a.url, '_blank', 'noopener,noreferrer')
      } else {
        window.location.assign(a.url)
      }
      return
    }
    void navigate(`/derslerim/${sectionId}/etkinlik/${a.id}`)
  }
  const ActionIcon = a.type === 'RESOURCE' ? DownloadIcon : a.type === 'URL' ? ExternalLinkIcon : undefined

  return (
    <li
      className={cn(
        'flex items-center gap-2 rounded-xl border border-transparent px-1 py-1 transition-colors hover:border-border hover:bg-muted/30',
        !a.is_visible && 'opacity-60',
      )}
    >
      <button
        type="button"
        onClick={activate}
        disabled={!open}
        className="flex min-w-0 flex-1 items-center gap-3 rounded-lg px-1 py-1 text-left outline-none focus-visible:ring-4 focus-visible:ring-ring/30 disabled:cursor-not-allowed"
      >
        <IconChip icon={meta.icon} tone={meta.tone} size="sm" />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-semibold">{a.title}</span>
          <span className="block truncate text-xs text-muted-foreground">{detail}</span>
        </span>
        {ActionIcon && open && <ActionIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
      </button>
      <AvailabilityBadges item={a} />
      {controls}
    </li>
  )
}
