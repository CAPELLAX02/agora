import { ArrowLeftIcon, DownloadIcon, ExternalLinkIcon, LockIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'

import { useGetActivityQuery, useGetSpaceQuery } from '@/shared/api/generated'
import { formatBytes } from '@/shared/api/upload'
import { formatDateTime } from '@/i18n/format'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Card, CardContent } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { RichText } from '@/shared/ui/rich-text'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { AssignmentStudent } from './AssignmentStudent'
import { AssignmentTeacher } from './AssignmentTeacher'
import { AvailabilityBadges } from './AvailabilityBadges'
import { ForumView } from './ForumView'
import { useDownload } from './download'
import { activityMeta } from './meta'

/**
 * ActivityPage, bir etkinliğin kendi sayfasıdır: sayfa içeriği, klasörün dosyaları ya da
 * dosyanın ve bağlantının ayrıntısı. Açık olmayan etkinlikte öğrenci sadece tarihini görür.
 */
export function ActivityPage() {
  const { t, i18n } = useTranslation()
  const { id = '', activityId = '' } = useParams()
  const localized = useLocalized()
  const download = useDownload()
  const { data: space } = useGetSpaceQuery({ id })
  const { data: a, error, refetch } = useGetActivityQuery({ id: activityId })
  const locale = i18n.language.startsWith('en') ? 'en-GB' : 'tr-TR'

  const back = (
    <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
      <Link to={`/derslerim/${id}`}>
        <ArrowLeftIcon />
        {space ? `${space.course.code}-${space.section_code}` : t('lms.mySpaces.title')}
      </Link>
    </Button>
  )

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!a) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-12 w-96" />
        <Skeleton className="h-64" />
      </div>
    )
  }

  const meta = activityMeta[a.type]
  const open = a.availability === 'OPEN' || space?.can_edit === true

  return (
    <>
      {back}
      <PageHeader
        icon={meta.icon}
        tone={meta.tone}
        title={a.title}
        description={
          space ? `${localized(space.course)} · ${t(`lms.types.${a.type}`)}` : t(`lms.types.${a.type}`)
        }
      />
      <div className="mb-4 flex flex-wrap gap-2">
        <AvailabilityBadges item={a} />
      </div>

      {!open ? (
        <Card>
          <CardContent className="flex items-center gap-3 py-6 text-muted-foreground">
            <LockIcon className="size-5" aria-hidden />
            {a.availability === 'NOT_YET' && a.available_from
              ? t('lms.activityNotYet', { time: formatDateTime(a.available_from) })
              : t('lms.activityClosed')}
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-4">
          {a.description_html && (
            <Card>
              <CardContent>
                <RichText html={a.description_html} />
              </CardContent>
            </Card>
          )}
          {a.type === 'PAGE' && (
            <Card>
              <CardContent>
                {a.body_html ? (
                  <RichText html={a.body_html} />
                ) : (
                  <p className="text-sm text-muted-foreground">{t('lms.pageEmpty')}</p>
                )}
              </CardContent>
            </Card>
          )}
          {(a.type === 'FOLDER' || a.type === 'RESOURCE') && (
            <Card>
              <CardContent>
                {a.files.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{t('lms.folderEmpty')}</p>
                ) : (
                  <ul className="divide-y">
                    {a.files.map((f) => (
                      <li key={f.id} className="flex items-center gap-3 py-2.5">
                        <span className="min-w-0 flex-1">
                          <span className="block truncate font-medium">{f.name}</span>
                          <span className="text-xs text-muted-foreground">{formatBytes(f.size, locale)}</span>
                        </span>
                        <Button size="sm" variant="soft" onClick={() => void download(a.id, f.id)}>
                          <DownloadIcon />
                          {t('lms.download')}
                        </Button>
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          )}
          {a.type === 'FORUM' && <ForumView sectionId={id} activityId={a.id} />}
          {a.type === 'ASSIGNMENT' &&
            space &&
            (space.role === 'STUDENT' ? (
              <AssignmentStudent activityId={a.id} />
            ) : (
              <AssignmentTeacher
                activityId={a.id}
                fileBase={`${space.course.code}-${space.section_code}_${a.title}`}
              />
            ))}
          {a.type === 'URL' && a.url && (
            <Button asChild>
              <a href={a.url} target={a.open_in_new_tab ? '_blank' : undefined} rel="noopener noreferrer">
                <ExternalLinkIcon />
                {t('lms.openLink')}
              </a>
            </Button>
          )}
        </div>
      )}
    </>
  )
}
