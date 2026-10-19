import { BellOffIcon, MessageCircleIcon, SearchIcon, StarIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'

import { formatRelative } from '@/i18n/format'
import { useListMyConversationsQuery } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { ConversationView } from './ConversationView'
import { NewConversationButton } from './NewConversationDialog'
import { conversationTitle } from './names'
import { ConversationIcon } from './parts'

/**
 * MessagesPage, mesajlaşma ekranıdır: solda konuşmalar, sağda açık konuşma. Dar ekranda konuşma
 * açıkken liste gizlenir.
 */
export function MessagesPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  return (
    <>
      <PageHeader
        icon={MessageCircleIcon}
        tone="sky"
        title={t('messages.title')}
        description={t('messages.description')}
        actions={<NewConversationButton />}
      />
      <div className="grid h-[calc(100svh-16rem)] min-h-[30rem] gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
        <ConversationList activeId={id} className={cn(id && 'hidden lg:flex')} />
        {id ? (
          <ConversationView key={id} id={id} />
        ) : (
          <div className="hidden items-center justify-center rounded-2xl border border-dashed border-border bg-card/50 lg:flex">
            <EmptyState icon={MessageCircleIcon} tone="sky" className="border-0">
              {t('messages.pick')}
            </EmptyState>
          </div>
        )}
      </div>
    </>
  )
}

function ConversationList({ activeId, className }: { activeId?: string; className?: string }) {
  const { t } = useTranslation()
  const { data, error, refetch } = useListMyConversationsQuery()
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<'all' | 'unread' | 'starred'>('all')
  const q = query.trim().toLocaleLowerCase('tr')
  const items = (data?.items ?? []).filter(
    (c) =>
      (!q || conversationTitle(c).toLocaleLowerCase('tr').includes(q)) &&
      (filter === 'all' || (filter === 'unread' ? c.unread > 0 : c.is_starred)),
  )
  return (
    <section
      aria-label={t('messages.conversations')}
      className={cn(
        'flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border/70 bg-card shadow-soft',
        className,
      )}
    >
      <div className="relative space-y-2 border-b border-border/60 p-3">
        <div className="flex gap-1" role="group" aria-label={t('messages.filter')}>
          {(['all', 'unread', 'starred'] as const).map((f) => (
            <button
              key={f}
              type="button"
              aria-pressed={filter === f}
              onClick={() => setFilter(f)}
              className={cn(
                'rounded-full px-2.5 py-1 text-xs font-semibold transition-colors',
                filter === f
                  ? 'bg-sky-500 text-white'
                  : 'bg-muted text-muted-foreground hover:text-foreground',
              )}
            >
              {t(`messages.filters.${f}`)}
            </button>
          ))}
        </div>
        <SearchIcon className="pointer-events-none absolute bottom-5.5 left-5.5 size-4 text-muted-foreground" />
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('messages.search')}
          aria-label={t('messages.search')}
          className="pl-8"
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-1.5">
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <div className="space-y-2 p-2">
            <Skeleton className="h-14" />
            <Skeleton className="h-14" />
          </div>
        ) : items.length === 0 ? (
          <p className="px-3 py-8 text-center text-sm text-muted-foreground">
            {q ? t('messages.noMatch') : t('messages.empty')}
          </p>
        ) : (
          <ul className="space-y-0.5">
            {items.map((c) => {
              const title = conversationTitle(c)
              const last = c.last_message
              return (
                <li key={c.id}>
                  <Link
                    to={`/mesajlar/${c.id}`}
                    aria-current={c.id === activeId ? 'page' : undefined}
                    className={cn(
                      'flex items-center gap-3 rounded-xl px-2.5 py-2 transition-colors outline-none hover:bg-muted/60 focus-visible:ring-4 focus-visible:ring-ring/30',
                      c.id === activeId && 'bg-sky-500/10 hover:bg-sky-500/15',
                    )}
                  >
                    <ConversationIcon c={c} />
                    <span className="min-w-0 flex-1">
                      <span className="flex items-baseline gap-2">
                        <span
                          className={cn(
                            'min-w-0 flex-1 truncate text-sm',
                            c.unread > 0 ? 'font-extrabold' : 'font-semibold',
                          )}
                        >
                          {title}
                        </span>
                        {c.is_starred && (
                          <StarIcon
                            className="size-3.5 shrink-0 fill-amber-400 text-amber-500"
                            aria-label={t('messages.starred')}
                          />
                        )}
                        {c.muted_until && new Date(c.muted_until) > new Date() && (
                          <BellOffIcon
                            className="size-3.5 shrink-0 text-muted-foreground"
                            aria-label={t('messages.muted')}
                          />
                        )}
                        {c.last_message_at && (
                          <span className="shrink-0 text-[11px] text-muted-foreground">
                            {formatRelative(c.last_message_at)}
                          </span>
                        )}
                      </span>
                      <span className="flex items-center gap-2">
                        <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                          {!last
                            ? t('messages.noMessages')
                            : last.deleted
                              ? t('messages.deleted')
                              : `${last.mine ? t('messages.you') : c.kind === 'DIRECT' ? '' : `${last.sender.name.split(' ')[0]}: `}${last.body || t('messages.attachmentOnly')}`}
                        </span>
                        {c.unread > 0 && (
                          <span
                            className="min-w-5 rounded-full bg-sky-500 px-1.5 text-center text-[11px] leading-5 font-bold text-white tabular-nums"
                            aria-label={t('messages.unread', { count: c.unread })}
                          >
                            {c.unread}
                          </span>
                        )}
                      </span>
                    </span>
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </section>
  )
}
