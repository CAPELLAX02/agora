import {
  ArrowLeftIcon,
  CornerDownRightIcon,
  LockIcon,
  LockOpenIcon,
  MessagesSquareIcon,
  PencilIcon,
  PinIcon,
  PinOffIcon,
  ReplyIcon,
  SendIcon,
  Trash2Icon,
  XIcon,
} from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import {
  useDeleteForumPostMutation,
  useDeleteForumThreadMutation,
  useEditForumPostMutation,
  useGetForumThreadQuery,
  useGetSpaceQuery,
  useReplyForumThreadMutation,
  useUpdateForumThreadMutation,
  type ForumThread,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader } from '@/shared/ui/page-header'
import { RichText } from '@/shared/ui/rich-text'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { Initials } from './forumParts'

// Zengin metin editörü yanıt ve düzenleme için gerektiğinde indirilir.
const RichTextEditor = lazy(() =>
  import('@/shared/ui/rich-text-editor').then((m) => ({ default: m.RichTextEditor })),
)

type Post = ForumThread['posts'][number]
type Pending = { kind: 'delete-post'; post: Post } | { kind: 'delete-thread' }

/**
 * ThreadPage, bir forum konusudur: gönderiler (eskiden yeniye, yanıtlanan gönderiyle), yanıt
 * kutusu, kendi gönderisini düzenleme ve silme; öğretim elemanı konuyu sabitler, kilitler, siler.
 */
export function ThreadPage() {
  const { t } = useTranslation()
  const { id = '', activityId = '', threadId = '' } = useParams()
  const navigate = useNavigate()
  const message = useErrorMessage()
  const { data: space } = useGetSpaceQuery({ id })
  const { data: thread, error, refetch } = useGetForumThreadQuery({ id: threadId })
  const [update] = useUpdateForumThreadMutation()
  const [removeThread] = useDeleteForumThreadMutation()
  const [removePost] = useDeleteForumPostMutation()
  const [replyTo, setReplyTo] = useState<Post | null>(null)
  const [pending, setPending] = useState<Pending | null>(null)
  const forumPath = `/derslerim/${id}/etkinlik/${activityId}`

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!thread) {
    return <Skeleton className="h-64" />
  }
  const byId = new Map(thread.posts.map((p) => [p.id, p]))
  const firstId = thread.posts[0]?.id

  const toggle = async (body: { is_pinned?: boolean; is_locked?: boolean }) => {
    const res = await update({ id: threadId, body })
    if (res.error) {
      toast.error(message(res.error))
    }
  }

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
        <Link to={forumPath}>
          <ArrowLeftIcon />
          {space
            ? `${space.course.code}-${space.section_code} · ${t('lms.forum.back')}`
            : t('lms.forum.back')}
        </Link>
      </Button>
      <PageHeader
        icon={MessagesSquareIcon}
        tone="emerald"
        title={thread.title}
        description={t('lms.forum.startedBy', {
          name: thread.author.name,
          time: formatDateTime(thread.created_at),
        })}
        actions={
          thread.can_moderate && (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void toggle({ is_pinned: !thread.is_pinned })}>
                {thread.is_pinned ? <PinOffIcon /> : <PinIcon />}
                {thread.is_pinned ? t('lms.forum.unpin') : t('lms.forum.pin')}
              </Button>
              <Button variant="outline" onClick={() => void toggle({ is_locked: !thread.is_locked })}>
                {thread.is_locked ? <LockOpenIcon /> : <LockIcon />}
                {thread.is_locked ? t('lms.forum.unlock') : t('lms.forum.lock')}
              </Button>
              <Button
                variant="outline"
                className="text-rose-600"
                onClick={() => setPending({ kind: 'delete-thread' })}
              >
                <Trash2Icon />
                {t('lms.forum.deleteThread')}
              </Button>
            </div>
          )
        }
      />
      <div className="mb-4 flex flex-wrap gap-2">
        {thread.is_pinned && (
          <Badge variant="warning" className="gap-1">
            <PinIcon className="size-3" />
            {t('lms.forum.pinned')}
          </Badge>
        )}
        {thread.is_locked && (
          <Badge variant="muted" className="gap-1">
            <LockIcon className="size-3" />
            {t('lms.forum.locked')}
          </Badge>
        )}
      </div>

      <ol className="space-y-3">
        {thread.posts.map((p) => (
          <PostCard
            key={p.id}
            post={p}
            parent={p.parent_post_id ? byId.get(p.parent_post_id) : undefined}
            canReply={thread.can_reply}
            onReply={() => {
              setReplyTo(p)
              document.getElementById('reply-box')?.scrollIntoView({ behavior: 'smooth', block: 'center' })
            }}
            onDelete={() => setPending({ kind: 'delete-post', post: p })}
          />
        ))}
      </ol>

      <div className="mt-6">
        {thread.can_reply ? (
          <ReplyBox
            key={replyTo?.id ?? 'root'}
            threadId={threadId}
            replyTo={replyTo}
            onCancelReplyTo={() => setReplyTo(null)}
            onSent={() => setReplyTo(null)}
          />
        ) : (
          <Alert>
            <LockIcon />
            <AlertDescription>
              {thread.is_locked ? t('lms.forum.lockedHint') : t('lms.forum.readOnlyHint')}
            </AlertDescription>
          </Alert>
        )}
      </div>

      {pending && (
        <ConfirmDialog
          title={
            pending.kind === 'delete-thread' || pending.post.id === firstId
              ? t('lms.forum.deleteThreadTitle')
              : t('lms.forum.deletePostTitle')
          }
          description={
            pending.kind === 'delete-thread' || pending.post.id === firstId
              ? t('lms.forum.deleteThreadDescription')
              : t('lms.forum.deletePostDescription')
          }
          confirm={t('lms.delete')}
          destructive
          onClose={() => setPending(null)}
          onConfirm={async () => {
            const deletingThread = pending.kind === 'delete-thread' || pending.post.id === firstId
            const res =
              pending.kind === 'delete-thread'
                ? await removeThread({ id: threadId })
                : await removePost({ id: pending.post.id })
            if (res.error) {
              toast.error(message(res.error))
              return
            }
            setPending(null)
            if (deletingThread) {
              toast.success(t('lms.forum.threadDeleted'))
              void navigate(forumPath)
            }
          }}
        />
      )}
    </>
  )
}

function PostCard({
  post,
  parent,
  canReply,
  onReply,
  onDelete,
}: {
  post: Post
  parent?: Post
  canReply: boolean
  onReply: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [edit, { isLoading }] = useEditForumPostMutation()
  const [editing, setEditing] = useState(false)
  const [body, setBody] = useState(post.body_html)

  const save = async () => {
    const res = await edit({ id: post.id, body: { body_html: body } })
    if (res.error) {
      toast.error(message(res.error))
      return
    }
    setEditing(false)
  }

  return (
    <li
      id={`post-${post.id}`}
      className={cn(
        'rounded-2xl border border-border/70 bg-card p-4 shadow-xs',
        post.author.is_instructor && 'border-violet-200/70 dark:border-violet-500/20',
      )}
    >
      <div className="flex items-start gap-3">
        <Initials name={post.author.name} instructor={post.author.is_instructor} />
        <div className="min-w-0 flex-1 space-y-2">
          <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
            <span className="font-semibold">{post.author.name}</span>
            {post.author.is_instructor && <Badge variant="violet">{t('lms.forum.instructor')}</Badge>}
            <span className="text-xs text-muted-foreground">{formatDateTime(post.created_at)}</span>
            {post.edited_at && !post.deleted && (
              <span className="text-xs text-muted-foreground" title={formatDateTime(post.edited_at)}>
                · {t('lms.forum.edited')}
              </span>
            )}
          </p>
          {parent && (
            <a
              href={`#post-${parent.id}`}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
            >
              <CornerDownRightIcon className="size-3" aria-hidden />
              {t('lms.forum.inReplyTo', { name: parent.author.name })}
            </a>
          )}
          {post.deleted ? (
            <p className="text-sm text-muted-foreground italic">{t('lms.forum.deleted')}</p>
          ) : editing ? (
            <div className="space-y-2">
              <Suspense fallback={<Skeleton className="h-32" />}>
                <RichTextEditor
                  value={body}
                  onChange={setBody}
                  label={t('lms.forum.editLabel')}
                  minHeight="min-h-28"
                />
              </Suspense>
              <div className="flex justify-end gap-2">
                <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
                  <XIcon />
                  {t('common.cancel')}
                </Button>
                <Button size="sm" onClick={() => void save()} disabled={isLoading}>
                  {t('common.save')}
                </Button>
              </div>
            </div>
          ) : (
            <RichText html={post.body_html} />
          )}
          {!post.deleted && !editing && (canReply || post.can_edit) && (
            <div className="flex flex-wrap gap-1">
              {canReply && (
                <Button size="sm" variant="ghost" onClick={onReply}>
                  <ReplyIcon />
                  {t('lms.forum.reply')}
                </Button>
              )}
              {post.can_edit && (
                <>
                  <Button size="sm" variant="ghost" onClick={() => setEditing(true)}>
                    <PencilIcon />
                    {t('lms.edit')}
                  </Button>
                  <Button size="sm" variant="ghost" className="text-rose-600" onClick={onDelete}>
                    <Trash2Icon />
                    {t('lms.delete')}
                  </Button>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </li>
  )
}

function ReplyBox({
  threadId,
  replyTo,
  onCancelReplyTo,
  onSent,
}: {
  threadId: string
  replyTo: Post | null
  onCancelReplyTo: () => void
  onSent: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [reply, { isLoading }] = useReplyForumThreadMutation()
  const [body, setBody] = useState('')

  const send = async () => {
    const res = await reply({ id: threadId, body: { body_html: body, parent_post_id: replyTo?.id ?? null } })
    if (res.error) {
      toast.error(message(res.error))
      return
    }
    setBody('')
    toast.success(t('lms.forum.replied'))
    onSent()
  }

  return (
    <section
      id="reply-box"
      aria-label={t('lms.forum.replyTitle')}
      className="space-y-2 rounded-2xl border border-emerald-200/70 bg-emerald-50/30 p-4 dark:border-emerald-500/20 dark:bg-emerald-500/5"
    >
      <h2 className="text-sm font-bold">{t('lms.forum.replyTitle')}</h2>
      {replyTo && (
        <p className="flex items-center gap-2 text-xs text-muted-foreground">
          <CornerDownRightIcon className="size-3" aria-hidden />
          {t('lms.forum.inReplyTo', { name: replyTo.author.name })}
          <button type="button" className="underline" onClick={onCancelReplyTo}>
            {t('lms.forum.cancelReplyTo')}
          </button>
        </p>
      )}
      <Suspense fallback={<Skeleton className="h-32" />}>
        <RichTextEditor
          value={body}
          onChange={setBody}
          label={t('lms.forum.replyLabel')}
          minHeight="min-h-28"
        />
      </Suspense>
      <div className="flex justify-end">
        <Button onClick={() => void send()} disabled={isLoading || !body.trim()}>
          <SendIcon />
          {t('lms.forum.send')}
        </Button>
      </div>
    </section>
  )
}
