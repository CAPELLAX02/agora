import { ArrowLeftIcon, BellIcon, BellOffIcon, LogOutIcon, StarIcon } from 'lucide-react'
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import {
  useGetConversationQuery,
  useLeaveConversationMutation,
  useListConversationMessagesQuery,
  useMarkConversationReadMutation,
  useSetConversationStateMutation,
  type Conversation,
  type ConversationMessage,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { Bubble } from './Bubble'
import { Composer } from './Composer'
import { conversationTitle } from './names'
import { ConversationIcon } from './parts'

const pageSize = 30

/**
 * ConversationView, açık konuşmadır: başlık (yıldız, sessiz, gruptan ayrılma), iletiler (en yeni
 * altta; yukarıda eski iletiler sayfa sayfa) ve yazma kutusu. Görüntülenen en yeni ileti okundu sayılır.
 */
export function ConversationView({ id }: { id: string }) {
  const { t } = useTranslation()
  const { data: c, error, refetch } = useGetConversationQuery({ id })
  const newest = useListConversationMessagesQuery({ id, limit: pageSize })
  // Eski sayfalar: her biri bir öncekinin en eski iletisinden önce (en yakını önce).
  const [older, setOlder] = useState<string[]>([])
  const [replyTo, setReplyTo] = useState<ConversationMessage | null>(null)
  const [markRead] = useMarkConversationReadMutation()
  const scroller = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)

  const latest = newest.data?.items[0]
  useEffect(() => {
    if (c && latest && c.unread > 0) {
      void markRead({ id, body: { message_id: latest.id } })
    }
  }, [c, latest, id, markRead])

  // Yeni ileti gelince (kullanıcı en alttaysa) aşağı kaydırılır.
  useLayoutEffect(() => {
    const el = scroller.current
    if (el && atBottom.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [latest?.id])

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!c) {
    return <Skeleton className="h-full" />
  }
  const title = conversationTitle(c)
  const group = c.kind !== 'DIRECT'
  const recent = newest.data?.items ?? []
  const bubble = (m: ConversationMessage, page: ConversationMessage[]) => (
    <Bubble
      key={m.id}
      m={m}
      group={group}
      moderator={c.is_moderator}
      quote={m.reply_to_id ? [...page, ...recent].find((x) => x.id === m.reply_to_id) : undefined}
      onReply={setReplyTo}
    />
  )
  return (
    <section
      aria-label={title}
      className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border/70 bg-card shadow-soft"
    >
      <header className="flex items-center gap-3 border-b border-border/60 px-4 py-3">
        <Button variant="ghost" size="icon" className="lg:hidden" asChild>
          <Link to="/mesajlar" aria-label={t('messages.back')}>
            <ArrowLeftIcon />
          </Link>
        </Button>
        <ConversationIcon c={c} className="size-10" />
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-bold">{title}</h2>
          <p className="text-xs text-muted-foreground">
            {c.kind === 'DIRECT'
              ? `${c.peer?.number ?? ''}${c.peer?.is_staff ? ` · ${t('messages.staff')}` : ''}`
              : t('messages.members', { count: c.member_count ?? 0 })}
          </p>
        </div>
        <ConversationActions c={c} />
      </header>
      <div
        ref={scroller}
        className="min-h-0 flex-1 overflow-y-auto px-4 py-3"
        onScroll={(e) => {
          const el = e.currentTarget
          atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
        }}
      >
        {newest.error ? (
          <ErrorState error={newest.error} onRetry={() => void newest.refetch()} />
        ) : !newest.data ? (
          <Skeleton className="h-40" />
        ) : (
          <ol aria-label={t('messages.history')} className="space-y-2">
            {[...older].reverse().map((before, i) => (
              <OlderPage
                key={before}
                id={id}
                before={before}
                render={bubble}
                onMore={i === 0 ? (next) => setOlder((o) => [...o, next]) : undefined}
              />
            ))}
            {older.length === 0 && newest.data.has_more && (
              <li className="text-center">
                <Button variant="soft" size="sm" onClick={() => setOlder([recent[recent.length - 1]!.id])}>
                  {t('messages.loadOlder')}
                </Button>
              </li>
            )}
            {recent.length === 0 && older.length === 0 ? (
              <li className="py-10 text-center text-sm text-muted-foreground">{t('messages.start')}</li>
            ) : (
              [...recent].reverse().map((m) => bubble(m, recent))
            )}
          </ol>
        )}
      </div>
      <Composer
        id={id}
        replyTo={replyTo}
        onCancelReply={() => setReplyTo(null)}
        onSent={() => {
          atBottom.current = true
          setReplyTo(null)
        }}
      />
    </section>
  )
}

/** OlderPage, before iletisinden önceki bir sayfadır; en üstteki sayfa daha eskisini yükletir. */
function OlderPage({
  id,
  before,
  render,
  onMore,
}: {
  id: string
  before: string
  render: (m: ConversationMessage, page: ConversationMessage[]) => ReactNode
  onMore?: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data } = useListConversationMessagesQuery({ id, before, limit: pageSize })
  if (!data) {
    return (
      <li>
        <Skeleton className="h-16" />
      </li>
    )
  }
  return (
    <>
      {onMore && data.has_more && (
        <li className="text-center">
          <Button variant="soft" size="sm" onClick={() => onMore(data.items[data.items.length - 1]!.id)}>
            {t('messages.loadOlder')}
          </Button>
        </li>
      )}
      {[...data.items].reverse().map((m) => render(m, data.items))}
    </>
  )
}

/** muteOptions, sessize alma süreleridir (saat). */
const muteOptions = [1, 8, 168] as const

/** ConversationActions, konuşmanın kişisel işlemleridir: yıldız, süreyle sessiz, gruptan ayrılma. */
function ConversationActions({ c }: { c: Conversation }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const errorMessage = useErrorMessage()
  const [setState] = useSetConversationStateMutation()
  const [leave] = useLeaveConversationMutation()
  const [confirmLeave, setConfirmLeave] = useState(false)
  const muted = c.muted_until !== null && new Date(c.muted_until) > new Date()

  const update = async (starred: boolean, mutedUntil: string | null, message: string) => {
    const res = await setState({ id: c.id, body: { is_starred: starred, muted_until: mutedUntil } })
    if (res.error) {
      toast.error(errorMessage(res.error))
      return
    }
    toast.success(message)
  }
  return (
    <div className="flex items-center gap-1">
      <Button
        variant="ghost"
        size="icon"
        aria-pressed={c.is_starred}
        aria-label={c.is_starred ? t('messages.unstar') : t('messages.star')}
        onClick={() =>
          void update(
            !c.is_starred,
            c.muted_until,
            c.is_starred ? t('messages.unstarred') : t('messages.starred'),
          )
        }
      >
        <StarIcon className={cn(c.is_starred && 'fill-amber-400 text-amber-500')} />
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" aria-label={t('messages.notificationsMenu')}>
            {muted ? <BellOffIcon className="text-muted-foreground" /> : <BellIcon />}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {muted && c.muted_until && (
            <>
              <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
                {t('messages.mutedUntil', { time: formatDateTime(c.muted_until) })}
              </DropdownMenuLabel>
              <DropdownMenuItem onSelect={() => void update(c.is_starred, null, t('messages.unmuted'))}>
                <BellIcon />
                {t('messages.unmute')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          {muteOptions.map((h) => (
            <DropdownMenuItem
              key={h}
              onSelect={() =>
                void update(
                  c.is_starred,
                  new Date(Date.now() + h * 3600_000).toISOString(),
                  t('messages.mutedToast'),
                )
              }
            >
              <BellOffIcon />
              {t(`messages.muteFor.${h}`)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      {c.kind === 'GROUP' && (
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('messages.leave')}
          onClick={() => setConfirmLeave(true)}
        >
          <LogOutIcon />
        </Button>
      )}
      {confirmLeave && (
        <ConfirmDialog
          destructive
          icon={LogOutIcon}
          title={t('messages.leaveTitle')}
          description={t('messages.leaveDescription')}
          confirm={t('messages.leave')}
          onClose={() => setConfirmLeave(false)}
          onConfirm={async () => {
            const res = await leave({ id: c.id })
            if (res.error) {
              toast.error(errorMessage(res.error))
              return
            }
            void navigate('/mesajlar')
          }}
        />
      )}
    </div>
  )
}
