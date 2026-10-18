import { LockIcon, MessageSquarePlusIcon, MessagesSquareIcon, PinIcon } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime, formatRelative } from '@/i18n/format'
import { useCreateForumThreadMutation, useListForumThreadsQuery } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
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
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Skeleton } from '@/shared/ui/skeleton'
import { Spinner } from '@/shared/ui/spinner'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { Initials } from './forumParts'

// Zengin metin editörü sadece konu açılırken indirilir.
const RichTextEditor = lazy(() =>
  import('@/shared/ui/rich-text-editor').then((m) => ({ default: m.RichTextEditor })),
)

/**
 * ForumView, bir forumun konularıdır: sabitlenenler önce, sonra son gönderiye göre; konu
 * açabilen "Yeni konu" ile başlık ve ilk gönderiyi yazar.
 */
export function ForumView({ sectionId, activityId }: { sectionId: string; activityId: string }) {
  const { t } = useTranslation()
  const { data, error, refetch } = useListForumThreadsQuery({ id: activityId })
  const [creating, setCreating] = useState(false)
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-48" />
  }
  const announcements = data.forum.kind === 'ANNOUNCEMENTS'
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <p className="flex-1 text-sm text-muted-foreground">
          {announcements ? t('lms.forum.announcementsHint') : t('lms.forum.generalHint')}
        </p>
        {data.can_start && (
          <Button onClick={() => setCreating(true)}>
            <MessageSquarePlusIcon />
            {announcements ? t('lms.forum.newAnnouncement') : t('lms.forum.newThread')}
          </Button>
        )}
      </div>
      {data.items.length === 0 ? (
        <EmptyState icon={MessagesSquareIcon} tone="emerald">
          {announcements ? t('lms.forum.noAnnouncements') : t('lms.forum.noThreads')}
        </EmptyState>
      ) : (
        <ul className="space-y-2">
          {data.items.map((th) => (
            <li key={th.id}>
              <Link
                to={`/derslerim/${sectionId}/etkinlik/${activityId}/konu/${th.id}`}
                className="flex items-center gap-3 rounded-2xl border border-border/70 bg-card px-4 py-3 shadow-xs transition-all hover:-translate-y-0.5 hover:shadow-soft"
              >
                <Initials name={th.author.name} instructor={th.author.is_instructor} />
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-center gap-1.5">
                    {th.is_pinned && (
                      <PinIcon className="size-3.5 text-amber-600" aria-label={t('lms.forum.pinned')} />
                    )}
                    {th.is_locked && (
                      <LockIcon
                        className="size-3.5 text-muted-foreground"
                        aria-label={t('lms.forum.locked')}
                      />
                    )}
                    <span className="truncate font-semibold">{th.title}</span>
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {th.author.name} · {formatDateTime(th.created_at)}
                  </span>
                </span>
                <span className="shrink-0 text-right text-xs text-muted-foreground">
                  <Badge variant="muted">{t('lms.forum.posts', { count: th.post_count })}</Badge>
                  <span className="mt-1 block" title={formatDateTime(th.last_post_at)}>
                    {t('lms.forum.lastPost', { time: formatRelative(th.last_post_at) })}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {creating && (
        <NewThreadDialog
          sectionId={sectionId}
          activityId={activityId}
          announcements={announcements}
          onClose={() => setCreating(false)}
        />
      )}
    </div>
  )
}

function NewThreadDialog({
  sectionId,
  activityId,
  announcements,
  onClose,
}: {
  sectionId: string
  activityId: string
  announcements: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const navigate = useNavigate()
  const [create, { isLoading }] = useCreateForumThreadMutation()
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    if (!title.trim() || !body.trim()) {
      setError(t('lms.forum.required'))
      return
    }
    const res = await create({ id: activityId, body: { title: title.trim(), body_html: body } })
    if (res.error) {
      setError(message(res.error))
      return
    }
    toast.success(t('lms.forum.created'))
    onClose()
    void navigate(`/derslerim/${sectionId}/etkinlik/${activityId}/konu/${res.data.id}`)
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl" closeLabel={t('common.close')}>
        <DialogHeader icon={MessageSquarePlusIcon} tone="emerald">
          <DialogTitle>
            {announcements ? t('lms.forum.newAnnouncement') : t('lms.forum.newThread')}
          </DialogTitle>
          <DialogDescription>{t('lms.forum.newHint')}</DialogDescription>
        </DialogHeader>
        <form
          noValidate
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="thread-title">{t('lms.forum.title')}</Label>
            <Input
              id="thread-title"
              value={title}
              maxLength={200}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <span className="text-sm font-medium">{t('lms.forum.message')}</span>
            <Suspense fallback={<Skeleton className="h-40" />}>
              <RichTextEditor
                value={body}
                onChange={setBody}
                label={t('lms.forum.message')}
                minHeight="min-h-40"
              />
            </Suspense>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading && <Spinner />}
              {t('lms.forum.publish')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
