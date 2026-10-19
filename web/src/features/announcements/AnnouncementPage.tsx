import { ArchiveIcon, ArrowLeftIcon, PencilIcon, SendIcon, Trash2Icon, UsersRoundIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  useArchiveAnnouncementMutation,
  useDeleteAnnouncementMutation,
  useGetAnnouncementQuery,
  usePublishAnnouncementMutation,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { IconChip } from '@/shared/ui/icon-chip'
import { Meter } from '@/shared/ui/meter'
import { RichText } from '@/shared/ui/rich-text'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { phaseBadge, phaseOf, typeMeta } from './meta'
import { AudienceList, PinnedMark, TypeBadge } from './parts'

/**
 * AnnouncementPage, bir duyurunun ayrıntısıdır. Hedef kitledeki kullanıcı açınca okundu sayılır;
 * yönetene durum, hedef kitle büyüklüğü, okuyan oranı ve işlemler (düzenle, yayımla, yayından
 * kaldır, sil) görünür.
 */
export function AnnouncementPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const errorMessage = useErrorMessage()
  const { data: a, error, refetch } = useGetAnnouncementQuery({ id })
  const [publish, publishing] = usePublishAnnouncementMutation()
  const [archive] = useArchiveAnnouncementMutation()
  const [remove] = useDeleteAnnouncementMutation()
  const [confirm, setConfirm] = useState<'archive' | 'delete' | null>(null)

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!a) {
    return <Skeleton className="h-96" />
  }
  const meta = typeMeta[a.type]
  const phase = phaseOf(a)
  const audience = a.stats?.audience ?? 0
  const done = (message: string, res: { error?: unknown }) => {
    if (res.error) {
      toast.error(errorMessage(res.error))
      return false
    }
    toast.success(message)
    return true
  }

  return (
    <div className="space-y-5">
      <Button variant="ghost" size="sm" asChild>
        <Link to={a.can_manage ? '/duyuru-yonetimi' : '/duyurular'}>
          <ArrowLeftIcon />
          {a.can_manage ? t('announcements.manage.title') : t('announcements.title')}
        </Link>
      </Button>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_300px]">
        <Card>
          <CardHeader className="flex items-start gap-4">
            <IconChip icon={meta.icon} tone={meta.tone} size="lg" solid />
            <div className="min-w-0 flex-1 space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <TypeBadge type={a.type} />
                {a.is_pinned && <PinnedMark />}
                {a.can_manage && (
                  <Badge variant={phaseBadge[phase]}>{t(`announcements.phases.${phase}`)}</Badge>
                )}
              </div>
              <h1 className="text-2xl font-bold tracking-tight">{a.title}</h1>
              <p className="text-sm text-muted-foreground">
                {a.author.name} · {formatDateTime(a.publish_from)}
                {a.publish_until && ` – ${formatDateTime(a.publish_until)}`}
              </p>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {a.summary && <p className="text-base font-medium">{a.summary}</p>}
            <RichText html={a.body_html} />
            <AudienceList audiences={a.audiences} />
          </CardContent>
        </Card>

        {a.can_manage && (
          <aside className="space-y-4">
            <Card>
              <CardHeader className="flex items-center gap-3">
                <IconChip icon={UsersRoundIcon} tone="amber" />
                <CardTitle>{t('announcements.stats.title')}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <p className="text-sm">{t('announcements.stats.audience', { count: audience })}</p>
                <Meter
                  value={a.stats?.reads ?? 0}
                  max={Math.max(audience, 1)}
                  tone="amber"
                  label={t('announcements.stats.readsLabel')}
                />
                <p className="text-sm text-muted-foreground">
                  {t('announcements.stats.reads', { count: a.stats?.reads ?? 0 })}
                </p>
                {a.notified_at && (
                  <p className="text-xs text-muted-foreground">
                    {t('announcements.stats.notified', { time: formatDateTime(a.notified_at) })}
                  </p>
                )}
              </CardContent>
            </Card>
            <div className="flex flex-col gap-2">
              {a.status !== 'ARCHIVED' && (
                <Button variant="outline" asChild>
                  <Link to={`/duyuru-yonetimi/${a.id}/duzenle`}>
                    <PencilIcon />
                    {t('announcements.actions.edit')}
                  </Link>
                </Button>
              )}
              {a.status === 'DRAFT' && (
                <Button
                  disabled={publishing.isLoading}
                  onClick={() =>
                    void publish({ id: a.id, 'If-Match': ifMatch(a.version) }).then((res) =>
                      done(t('announcements.actions.published'), res),
                    )
                  }
                >
                  <SendIcon />
                  {t('announcements.actions.publish')}
                </Button>
              )}
              {a.status === 'PUBLISHED' && (
                <Button variant="outline" onClick={() => setConfirm('archive')}>
                  <ArchiveIcon />
                  {t('announcements.actions.archive')}
                </Button>
              )}
              {a.status === 'DRAFT' && (
                <Button variant="outline" onClick={() => setConfirm('delete')}>
                  <Trash2Icon />
                  {t('announcements.actions.delete')}
                </Button>
              )}
            </div>
          </aside>
        )}
      </div>

      {confirm === 'archive' && (
        <ConfirmDialog
          icon={ArchiveIcon}
          tone="amber"
          title={t('announcements.actions.archiveTitle')}
          description={t('announcements.actions.archiveDescription')}
          confirm={t('announcements.actions.archive')}
          onClose={() => setConfirm(null)}
          onConfirm={async () => {
            const res = await archive({ id: a.id, 'If-Match': ifMatch(a.version) })
            if (done(t('announcements.actions.archived'), res)) {
              setConfirm(null)
            }
          }}
        />
      )}
      {confirm === 'delete' && (
        <ConfirmDialog
          destructive
          title={t('announcements.actions.deleteTitle')}
          description={t('announcements.actions.deleteDescription')}
          confirm={t('announcements.actions.delete')}
          onClose={() => setConfirm(null)}
          onConfirm={async () => {
            const res = await remove({ id: a.id })
            if (done(t('announcements.actions.deleted'), res)) {
              void navigate('/duyuru-yonetimi')
            }
          }}
        />
      )}
    </div>
  )
}
