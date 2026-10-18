import {
  ArchiveIcon,
  ArchiveRestoreIcon,
  BellIcon,
  CheckCheckIcon,
  CheckIcon,
  SlidersHorizontalIcon,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime, formatRelative } from '@/i18n/format'
import {
  useArchiveMyNotificationMutation,
  useListMyNotificationsQuery,
  useMarkAllMyNotificationsReadMutation,
  useMarkMyNotificationReadMutation,
  useUnarchiveMyNotificationMutation,
  type Notification,
} from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { categoryMeta, notificationText } from './meta'

type Filter = 'all' | 'unread' | 'archived'

const pageSize = 20

/** NotificationsPage, kullanıcının bütün bildirimleridir: okunmamış ve arşiv süzgeci, sayfa sayfa. */
export function NotificationsPage() {
  const { t } = useTranslation()
  const [filter, setFilter] = useState<Filter>('all')
  // Sayfalar: her biri bir öncekinin next_cursor'ıyla açılır; süzgeç değişince baştan.
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined])
  const [markAllRead, markingAll] = useMarkAllMyNotificationsReadMutation()
  const change = (f: string) => {
    setFilter(f as Filter)
    setCursors([undefined])
  }
  return (
    <>
      <PageHeader
        icon={BellIcon}
        tone="rose"
        title={t('notifications.title')}
        description={t('notifications.description')}
        actions={
          <>
            <Button variant="outline" asChild>
              <Link to="/bildirimler/tercihler">
                <SlidersHorizontalIcon />
                {t('notifications.preferences.short')}
              </Link>
            </Button>
            <Button
              disabled={markingAll.isLoading}
              onClick={() =>
                void markAllRead().then((res) => {
                  if (res.data) {
                    toast.success(t('notifications.allRead', { count: res.data.updated }))
                  }
                })
              }
            >
              <CheckCheckIcon />
              {t('notifications.markAllRead')}
            </Button>
          </>
        }
      />
      <Tabs value={filter} onValueChange={change} className="mb-4">
        <TabsList>
          <TabsTrigger value="all">{t('notifications.filters.all')}</TabsTrigger>
          <TabsTrigger value="unread">{t('notifications.filters.unread')}</TabsTrigger>
          <TabsTrigger value="archived">{t('notifications.filters.archived')}</TabsTrigger>
        </TabsList>
      </Tabs>
      <Card className="divide-y divide-border/60 overflow-hidden p-0">
        {cursors.map((cursor, i) => (
          <NotificationPage
            key={`${filter}:${cursor ?? ''}`}
            filter={filter}
            cursor={cursor}
            first={i === 0}
            onMore={i === cursors.length - 1 ? (next) => setCursors((c) => [...c, next]) : undefined}
          />
        ))}
      </Card>
    </>
  )
}

function NotificationPage({
  filter,
  cursor,
  first,
  onMore,
}: {
  filter: Filter
  cursor?: string
  first: boolean
  onMore?: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data, error, refetch } = useListMyNotificationsQuery({
    unread: filter === 'unread' || undefined,
    archived: filter === 'archived' || undefined,
    limit: pageSize,
    cursor,
  })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return (
      <div className="space-y-3 p-4">
        <Skeleton className="h-14" />
        <Skeleton className="h-14" />
      </div>
    )
  }
  if (first && data.items.length === 0) {
    return (
      <EmptyState icon={BellIcon} tone="rose" className="border-0">
        {filter === 'unread'
          ? t('notifications.emptyUnread')
          : filter === 'archived'
            ? t('notifications.emptyArchived')
            : t('notifications.empty')}
      </EmptyState>
    )
  }
  return (
    <>
      <ul aria-label={first ? t('notifications.title') : undefined} className="divide-y divide-border/60">
        {data.items.map((n) => (
          <NotificationRow key={n.id} n={n} />
        ))}
      </ul>
      {onMore && data.next_cursor && (
        <div className="p-3 text-center">
          <Button variant="soft" size="sm" onClick={() => onMore(data.next_cursor!)}>
            {t('notifications.loadMore')}
          </Button>
        </div>
      )}
    </>
  )
}

function NotificationRow({ n }: { n: Notification }) {
  const { t, i18n } = useTranslation()
  const { title, body } = notificationText(n, i18n.language.startsWith('en'))
  const meta = categoryMeta[n.category]
  const [markRead] = useMarkMyNotificationReadMutation()
  const [archive] = useArchiveMyNotificationMutation()
  const [unarchive] = useUnarchiveMyNotificationMutation()
  return (
    <li className={cn('flex items-start gap-3 px-4 py-3.5', !n.read_at && 'bg-rose-500/[0.04]')}>
      <IconChip icon={meta.icon} tone={meta.tone} className="mt-0.5" />
      <div className="min-w-0 flex-1">
        <p className={cn('text-sm leading-snug', !n.read_at && 'font-bold')}>
          {n.link ? (
            <Link
              to={n.link}
              className="hover:underline"
              onClick={() => !n.read_at && void markRead({ id: n.id })}
            >
              {title}
            </Link>
          ) : (
            title
          )}
        </p>
        {body && <p className="mt-0.5 text-sm text-muted-foreground">{body}</p>}
        <p className="mt-1 text-xs text-muted-foreground" title={formatDateTime(n.created_at)}>
          {formatRelative(n.created_at)}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {!n.read_at && (
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label={t('notifications.markReadFor', { title })}
            onClick={() => void markRead({ id: n.id })}
          >
            <CheckIcon />
          </Button>
        )}
        {n.archived_at ? (
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label={t('notifications.unarchiveFor', { title })}
            onClick={() => void unarchive({ id: n.id })}
          >
            <ArchiveRestoreIcon />
          </Button>
        ) : (
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label={t('notifications.archiveFor', { title })}
            onClick={() => void archive({ id: n.id })}
          >
            <ArchiveIcon />
          </Button>
        )}
      </div>
    </li>
  )
}
