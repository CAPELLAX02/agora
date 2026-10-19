import { ArrowLeftIcon, BellRingIcon, LockIcon } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import {
  useGetMyNotificationPreferencesQuery,
  useSetMyNotificationPreferencesMutation,
  type NotificationCategory,
  type NotificationPreference,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { Checkbox } from '@/shared/ui/checkbox'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { categoryMeta } from './meta'

type Channels = { in_app: boolean; email: boolean }

/**
 * NotificationPreferencesPage, bildirim tercih matrisidir: tür başına uygulama içi ve e-posta.
 * Değişiklikler birlikte kaydedilir; güvenlik bildirimleri kapatılamaz.
 */
export function NotificationPreferencesPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const errorMessage = useErrorMessage()
  const { data, error, refetch } = useGetMyNotificationPreferencesQuery()
  const [save, saving] = useSetMyNotificationPreferencesMutation()
  const [edits, setEdits] = useState<Record<string, Channels>>({})

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  const value = (p: NotificationPreference): Channels => edits[p.type] ?? { in_app: p.in_app, email: p.email }
  const changed = (data?.items ?? []).filter((p) => {
    const v = value(p)
    return v.in_app !== p.in_app || v.email !== p.email
  })
  const set = (p: NotificationPreference, channel: keyof Channels, on: boolean) =>
    setEdits((e) => ({ ...e, [p.type]: { ...value(p), [channel]: on } }))
  const submit = async () => {
    const res = await save({ body: { items: changed.map((p) => ({ type: p.type, ...value(p) })) } })
    if (res.error) {
      toast.error(errorMessage(res.error))
      return
    }
    setEdits({})
    toast.success(t('notifications.preferences.saved'))
  }

  // Türler katalog sırasıyla gelir; kategoriler ilk görüldükleri sırayla gruplanır.
  const groups: { category: NotificationCategory; items: NotificationPreference[] }[] = []
  for (const p of data?.items ?? []) {
    const g = groups.find((x) => x.category === p.category)
    if (g) {
      g.items.push(p)
    } else {
      groups.push({ category: p.category, items: [p] })
    }
  }

  return (
    <>
      <PageHeader
        icon={BellRingIcon}
        tone="violet"
        title={t('notifications.preferences.title')}
        description={t('notifications.preferences.description')}
        actions={
          <Button variant="outline" asChild>
            <Link to="/bildirimler">
              <ArrowLeftIcon />
              {t('notifications.title')}
            </Link>
          </Button>
        }
      />
      {!data ? (
        <Skeleton className="h-96" />
      ) : (
        <Card className="overflow-hidden p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('notifications.preferences.type')}</TableHead>
                <TableHead className="w-20 text-center whitespace-normal sm:w-32">
                  {t('notifications.preferences.inApp')}
                </TableHead>
                <TableHead className="w-20 text-center whitespace-normal sm:w-32">
                  {t('notifications.preferences.email')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((g) => {
                const meta = categoryMeta[g.category]
                return (
                  <Fragment key={g.category}>
                    <TableRow className="bg-muted/40 hover:bg-muted/40">
                      <TableCell colSpan={3} className="py-2">
                        <span className="flex items-center gap-2 font-bold">
                          <IconChip icon={meta.icon} tone={meta.tone} size="sm" />
                          {t(`notifications.categories.${g.category}`)}
                        </span>
                      </TableCell>
                    </TableRow>
                    {g.items.map((p) => {
                      const v = value(p)
                      const name = localized(p)
                      return (
                        <TableRow key={p.type}>
                          <TableCell className="whitespace-normal">
                            <span className="flex flex-wrap items-center gap-2">
                              {name}
                              {p.mandatory && (
                                <Badge
                                  variant="secondary"
                                  title={t('notifications.preferences.mandatoryHint')}
                                >
                                  <LockIcon />
                                  {t('notifications.preferences.mandatory')}
                                </Badge>
                              )}
                            </span>
                          </TableCell>
                          {(['in_app', 'email'] as const).map((channel) => (
                            <TableCell key={channel} className="text-center">
                              <Checkbox
                                checked={v[channel]}
                                disabled={p.mandatory}
                                onCheckedChange={(on) => set(p, channel, on === true)}
                                aria-label={`${name}: ${t(channel === 'in_app' ? 'notifications.preferences.inApp' : 'notifications.preferences.email')}`}
                              />
                            </TableCell>
                          ))}
                        </TableRow>
                      )
                    })}
                  </Fragment>
                )
              })}
            </TableBody>
          </Table>
          <div className="flex items-center justify-end gap-3 border-t border-border/60 p-4">
            {changed.length > 0 && (
              <span className="text-sm text-muted-foreground">
                {t('notifications.preferences.unsaved', { count: changed.length })}
              </span>
            )}
            <Button disabled={changed.length === 0 || saving.isLoading} onClick={() => void submit()}>
              {t('notifications.preferences.save')}
            </Button>
          </div>
        </Card>
      )}
    </>
  )
}
