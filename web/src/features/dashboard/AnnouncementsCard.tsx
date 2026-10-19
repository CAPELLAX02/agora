import { ArrowRightIcon, MegaphoneIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { PinnedMark } from '@/features/announcements/parts'
import { typeMeta } from '@/features/announcements/meta'
import { formatRelative } from '@/i18n/format'
import { useListMyAnnouncementsQuery } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'
import { Skeleton } from '@/shared/ui/skeleton'

import { DashCard } from './DashCard'

/** AnnouncementsCard, yayındaki son duyurulardır (sabitlenenler önce); okunmamışlar kalın. */
export function AnnouncementsCard() {
  const { t } = useTranslation()
  const { data, isError } = useListMyAnnouncementsQuery({ limit: 4 })
  if (isError) {
    return null
  }
  return (
    <DashCard
      icon={MegaphoneIcon}
      tone="amber"
      title={t('dashboard.announcements.title')}
      description={t('dashboard.announcements.description')}
    >
      {!data ? (
        <Skeleton className="h-28" />
      ) : data.items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('dashboard.announcements.empty')}</p>
      ) : (
        <ul className="grid grid-cols-[minmax(0,1fr)] gap-2">
          {data.items.map((a) => {
            const meta = typeMeta[a.type]
            return (
              <li key={a.id}>
                <Link
                  to={`/duyurular/${a.id}`}
                  className="group flex min-w-0 items-center gap-3 rounded-xl border border-border/70 px-3 py-2.5 text-sm transition-all hover:border-primary/25 hover:bg-primary/[0.03]"
                >
                  <IconChip icon={meta.icon} tone={meta.tone} size="sm" />
                  <span className="min-w-0 flex-1">
                    <span className={cn('block truncate', a.read_at ? 'font-semibold' : 'font-bold')}>
                      {a.title}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {a.author.name} · {formatRelative(a.publish_from)}
                    </span>
                  </span>
                  {a.is_pinned && <PinnedMark />}
                </Link>
              </li>
            )
          })}
        </ul>
      )}
      <Button variant="soft" size="sm" className="mt-auto self-start" asChild>
        <Link to="/duyurular">
          {t('dashboard.announcements.all')}
          <ArrowRightIcon />
        </Link>
      </Button>
    </DashCard>
  )
}
