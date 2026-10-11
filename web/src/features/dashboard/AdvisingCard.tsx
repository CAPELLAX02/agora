import { ArrowRightIcon, UsersRoundIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { useListAddDropQueueQuery, useListAdviseeRegistrationsQuery } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Skeleton } from '@/shared/ui/skeleton'

/**
 * AdvisingCard, danışmanın ve bölüm başkanının bekleyen işleridir: onay bekleyen ders kayıtları
 * ve karar bekleyen ekle-bırak talepleri.
 */
export function AdvisingCard() {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const advisor = has('registration:approve')
  const regs = useListAdviseeRegistrationsQuery({ status: 'SUBMITTED' }, { skip: !advisor })
  const queue = useListAddDropQueueQuery({})
  if (regs.isError && queue.isError) {
    return null
  }
  const rows = [
    ...(advisor
      ? [{ to: '/danismanlik', label: t('dashboard.advising.pending'), count: regs.data?.items.length }]
      : []),
    { to: '/ekle-birak-onaylari', label: t('dashboard.advising.addDrop'), count: queue.data?.items.length },
  ]
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <UsersRoundIcon className="size-4 text-primary" aria-hidden />
          {t('dashboard.advising.title')}
        </CardTitle>
        <CardDescription>{t('dashboard.advising.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="divide-y">
          {rows.map((r) => (
            <li key={r.to}>
              <Link
                to={r.to}
                className="flex items-center gap-3 py-2.5 text-sm font-medium hover:text-primary"
              >
                <span className="flex-1">{r.label}</span>
                {r.count === undefined ? (
                  <Skeleton className="h-5 w-8" />
                ) : (
                  <Badge variant={r.count > 0 ? 'warning' : 'muted'}>{r.count}</Badge>
                )}
                <ArrowRightIcon className="size-4 text-muted-foreground" aria-hidden />
              </Link>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
