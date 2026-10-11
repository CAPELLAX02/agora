import { ArrowRightIcon, ListChecksIcon, UserCheckIcon, UsersRoundIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { useListAddDropQueueQuery, useListAdviseeRegistrationsQuery } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { IconChip } from '@/shared/ui/icon-chip'
import { Skeleton } from '@/shared/ui/skeleton'

import { DashCard } from './DashCard'

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
      ? [
          {
            to: '/danismanlik',
            label: t('dashboard.advising.pending'),
            count: regs.data?.items.length,
            icon: UserCheckIcon,
            tone: 'violet' as const,
          },
        ]
      : []),
    {
      to: '/ekle-birak-onaylari',
      label: t('dashboard.advising.addDrop'),
      count: queue.data?.items.length,
      icon: ListChecksIcon,
      tone: 'teal' as const,
    },
  ]
  return (
    <DashCard
      icon={UsersRoundIcon}
      tone="violet"
      title={t('dashboard.advising.title')}
      description={t('dashboard.advising.description')}
    >
      <ul className="grid gap-2">
        {rows.map((r) => (
          <li key={r.to}>
            <Link
              to={r.to}
              className="group flex items-center gap-3 rounded-xl border border-border/70 px-3 py-2.5 text-sm font-semibold transition-all hover:border-primary/25 hover:bg-primary/[0.03]"
            >
              <IconChip icon={r.icon} tone={r.tone} size="sm" />
              <span className="flex-1">{r.label}</span>
              {r.count === undefined ? (
                <Skeleton className="h-6 w-8" />
              ) : (
                <span
                  className={cn(
                    'tabular inline-flex min-w-7 justify-center rounded-full px-2 py-0.5 text-sm font-extrabold',
                    r.count > 0
                      ? 'bg-amber-500/15 text-amber-800 dark:text-amber-300'
                      : 'bg-muted text-muted-foreground',
                  )}
                >
                  {r.count}
                </span>
              )}
              <ArrowRightIcon
                className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                aria-hidden
              />
            </Link>
          </li>
        ))}
      </ul>
    </DashCard>
  )
}
