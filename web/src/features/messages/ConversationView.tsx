import { ArrowLeftIcon, SendIcon } from 'lucide-react'
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import {
  useGetConversationQuery,
  useListConversationMessagesQuery,
  useMarkConversationReadMutation,
  useSendMessageMutation,
  type ConversationMessage,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Textarea } from '@/shared/ui/textarea'

import { conversationTitle } from './names'
import { ConversationIcon } from './parts'

const pageSize = 30
const maxBody = 4000

/**
 * ConversationView, açık konuşmadır: başlık, iletiler (en yeni altta; yukarıda eski iletiler sayfa
 * sayfa) ve yazma kutusu. Görüntülenen en yeni ileti okundu sayılır.
 */
export function ConversationView({ id }: { id: string }) {
  const { t } = useTranslation()
  const { data: c, error, refetch } = useGetConversationQuery({ id })
  const newest = useListConversationMessagesQuery({ id, limit: pageSize })
  // Eski sayfalar: her biri bir öncekinin en eski iletisinden önce (en yakını önce).
  const [older, setOlder] = useState<string[]>([])
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
  const oldestLoaded = older.length > 0 ? older[older.length - 1] : undefined
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
        <div className="min-w-0">
          <h2 className="truncate font-bold">{title}</h2>
          <p className="text-xs text-muted-foreground">
            {c.kind === 'DIRECT'
              ? `${c.peer?.number ?? ''}${c.peer?.is_staff ? ` · ${t('messages.staff')}` : ''}`
              : t('messages.members', { count: c.member_count ?? 0 })}
          </p>
        </div>
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
                group={c.kind !== 'DIRECT'}
                onMore={i === 0 ? (next) => setOlder((o) => [...o, next]) : undefined}
              />
            ))}
            {older.length === 0 && newest.data.has_more && (
              <li className="text-center">
                <Button
                  variant="soft"
                  size="sm"
                  onClick={() => setOlder([newest.data.items[newest.data.items.length - 1]!.id])}
                >
                  {t('messages.loadOlder')}
                </Button>
              </li>
            )}
            {newest.data.items.length === 0 && oldestLoaded === undefined ? (
              <li className="py-10 text-center text-sm text-muted-foreground">{t('messages.start')}</li>
            ) : (
              [...newest.data.items]
                .reverse()
                .map((m) => <Bubble key={m.id} m={m} group={c.kind !== 'DIRECT'} />)
            )}
          </ol>
        )}
      </div>
      <Composer id={id} onSent={() => (atBottom.current = true)} />
    </section>
  )
}

/** OlderPage, before iletisinden önceki bir sayfadır; en üstteki sayfa daha eskisini yükletir. */
function OlderPage({
  id,
  before,
  group,
  onMore,
}: {
  id: string
  before: string
  group: boolean
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
      {[...data.items].reverse().map((m) => (
        <Bubble key={m.id} m={m} group={group} />
      ))}
    </>
  )
}

function Bubble({ m, group }: { m: ConversationMessage; group: boolean }) {
  const { t } = useTranslation()
  return (
    <li className={cn('flex', m.mine ? 'justify-end' : 'justify-start')}>
      <div
        className={cn(
          'max-w-[80%] rounded-2xl px-3.5 py-2 text-sm shadow-xs',
          m.mine
            ? 'rounded-br-md bg-gradient-to-br from-sky-500 to-blue-600 text-white'
            : 'rounded-bl-md border border-border/70 bg-muted/40',
        )}
      >
        {group && !m.mine && (
          <p
            className={cn(
              'mb-0.5 text-xs font-bold',
              m.sender.is_staff ? 'text-violet-600 dark:text-violet-300' : 'text-sky-700 dark:text-sky-300',
            )}
          >
            {m.sender.name}
          </p>
        )}
        {m.deleted ? (
          <p className="italic opacity-75">{t('messages.deleted')}</p>
        ) : (
          <p className="break-words whitespace-pre-wrap">{m.body}</p>
        )}
        <p className={cn('mt-1 text-right text-[10px]', m.mine ? 'text-white/75' : 'text-muted-foreground')}>
          {formatDateTime(m.created_at)}
          {m.edited_at && ` · ${t('messages.edited')}`}
        </p>
      </div>
    </li>
  )
}

function Composer({ id, onSent }: { id: string; onSent: () => void }) {
  const { t } = useTranslation()
  const errorMessage = useErrorMessage()
  const [send, sending] = useSendMessageMutation()
  const [body, setBody] = useState('')
  const submit = async () => {
    const text = body.trim()
    if (!text || sending.isLoading) {
      return
    }
    const res = await send({ id, body: { body: text } })
    if (res.error) {
      toast.error(errorMessage(res.error))
      return
    }
    setBody('')
    onSent()
  }
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter gönderir, Shift+Enter satır atlatır.
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void submit()
    }
  }
  return (
    <form
      className="flex items-end gap-2 border-t border-border/60 p-3"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <Textarea
        value={body}
        maxLength={maxBody}
        rows={1}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={t('messages.placeholder')}
        aria-label={t('messages.compose')}
        className="max-h-40 min-h-10 resize-none"
      />
      <Button
        type="submit"
        size="icon"
        disabled={!body.trim() || sending.isLoading}
        aria-label={t('messages.send')}
      >
        <SendIcon />
      </Button>
    </form>
  )
}
