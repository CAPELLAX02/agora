import { EyeIcon, MegaphoneIcon, PlusIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { formatDateTime } from '@/i18n/format'
import { useListManagedAnnouncementsQuery } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { phaseBadge, phaseOf, typeMeta } from './meta'
import { AudienceList, PinnedMark } from './parts'

/** ManageAnnouncementsPage, yayımlayanın yönettiği duyurulardır: durum, hedef ve okuyan sayısı. */
export function ManageAnnouncementsPage() {
  const { t } = useTranslation()
  const { data, error, refetch } = useListManagedAnnouncementsQuery()
  return (
    <>
      <PageHeader
        icon={MegaphoneIcon}
        tone="orange"
        title={t('announcements.manage.title')}
        description={t('announcements.manage.description')}
        actions={
          <Button asChild>
            <Link to="/duyuru-yonetimi/yeni">
              <PlusIcon />
              {t('announcements.manage.new')}
            </Link>
          </Button>
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !data ? (
        <Skeleton className="h-64" />
      ) : data.items.length === 0 ? (
        <EmptyState icon={MegaphoneIcon} tone="orange">
          {t('announcements.manage.empty')}
        </EmptyState>
      ) : (
        <Card className="divide-y divide-border/60 overflow-hidden p-0">
          <ul aria-label={t('announcements.manage.title')} className="divide-y divide-border/60">
            {data.items.map((a) => {
              const meta = typeMeta[a.type]
              const phase = phaseOf(a)
              return (
                <li key={a.id} className="flex items-start gap-3 px-4 py-3.5">
                  <IconChip icon={meta.icon} tone={meta.tone} className="mt-0.5" />
                  <div className="min-w-0 flex-1 space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <Link to={`/duyurular/${a.id}`} className="font-semibold hover:underline">
                        {a.title}
                      </Link>
                      {a.is_pinned && <PinnedMark />}
                      <Badge variant={phaseBadge[phase]}>{t(`announcements.phases.${phase}`)}</Badge>
                    </div>
                    <AudienceList audiences={a.audiences} />
                    <p className="text-xs text-muted-foreground">
                      {a.author.name} · {formatDateTime(a.publish_from)}
                    </p>
                  </div>
                  <span
                    className="flex shrink-0 items-center gap-1 text-sm text-muted-foreground tabular-nums"
                    title={t('announcements.stats.reads', { count: a.stats?.reads ?? 0 })}
                  >
                    <EyeIcon className="size-4" aria-hidden />
                    {a.stats?.reads ?? 0}
                  </span>
                </li>
              )
            })}
          </ul>
        </Card>
      )}
    </>
  )
}
