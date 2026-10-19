import { MegaphoneIcon, PlusIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { formatDateTime, formatRelative } from '@/i18n/format'
import { useListMyAnnouncementsQuery, type Announcement } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { typeMeta } from './meta'
import { PinnedMark, TypeBadge } from './parts'

const pageSize = 20

/** AnnouncementsPage, kullanıcının hedef kitlesinde olduğu yayındaki duyurulardır. */
export function AnnouncementsPage() {
  const { t } = useTranslation()
  const { hasScoped } = usePermissions()
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined])
  return (
    <>
      <PageHeader
        icon={MegaphoneIcon}
        tone="amber"
        title={t('announcements.title')}
        description={t('announcements.description')}
        actions={
          hasScoped('announcement:publish') && (
            <Button variant="outline" asChild>
              <Link to="/duyuru-yonetimi">
                <PlusIcon />
                {t('announcements.manage.title')}
              </Link>
            </Button>
          )
        }
      />
      <div className="space-y-3">
        {cursors.map((cursor, i) => (
          <FeedPage
            key={cursor ?? ''}
            cursor={cursor}
            first={i === 0}
            onMore={i === cursors.length - 1 ? (next) => setCursors((c) => [...c, next]) : undefined}
          />
        ))}
      </div>
    </>
  )
}

function FeedPage({
  cursor,
  first,
  onMore,
}: {
  cursor?: string
  first: boolean
  onMore?: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data, error, refetch } = useListMyAnnouncementsQuery({ limit: pageSize, cursor })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-32" />
  }
  if (first && data.items.length === 0) {
    return (
      <EmptyState icon={MegaphoneIcon} tone="amber">
        {t('announcements.empty')}
      </EmptyState>
    )
  }
  return (
    <>
      <ul className="space-y-3" aria-label={first ? t('announcements.title') : undefined}>
        {data.items.map((a) => (
          <AnnouncementItem key={a.id} a={a} />
        ))}
      </ul>
      {onMore && data.next_cursor && (
        <div className="text-center">
          <Button variant="soft" size="sm" onClick={() => onMore(data.next_cursor!)}>
            {t('announcements.loadMore')}
          </Button>
        </div>
      )}
    </>
  )
}

function AnnouncementItem({ a }: { a: Announcement }) {
  const { t } = useTranslation()
  const meta = typeMeta[a.type]
  const unread = !a.read_at
  return (
    <li>
      <Link
        to={`/duyurular/${a.id}`}
        className={cn(
          'group flex gap-4 rounded-xl border border-border/70 bg-card p-4 shadow-soft focus-ring transition-all hover:-translate-y-0.5 hover:shadow-lift',
          a.type === 'URGENT' && 'border-rose-500/40',
        )}
      >
        <IconChip icon={meta.icon} tone={meta.tone} solid />
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <TypeBadge type={a.type} />
            {a.is_pinned && <PinnedMark />}
            {unread && <Badge variant="destructive">{t('announcements.new')}</Badge>}
          </div>
          <h2
            className={cn(
              'text-base leading-snug group-hover:underline',
              unread ? 'font-bold' : 'font-semibold',
            )}
          >
            {a.title}
          </h2>
          {a.summary && <p className="line-clamp-2 text-sm text-muted-foreground">{a.summary}</p>}
          <p className="text-xs text-muted-foreground" title={formatDateTime(a.publish_from)}>
            {a.author.name} · {formatRelative(a.publish_from)}
          </p>
        </div>
      </Link>
    </li>
  )
}
