import { ArrowRightIcon, BellIcon, CheckCheckIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { formatRelative } from '@/i18n/format'
import {
  agoraApi,
  useGetMyUnreadNotificationCountQuery,
  useListMyNotificationsQuery,
  useMarkAllMyNotificationsReadMutation,
  useMarkMyNotificationReadMutation,
  type Notification,
} from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { IconChip } from '@/shared/ui/icon-chip'
import { Skeleton } from '@/shared/ui/skeleton'

import { categoryMeta, notificationText } from './meta'
import { useNotificationStream } from './useNotificationStream'

/** recentLimit, zil menüsünde gösterilen son bildirim sayısıdır. */
const recentLimit = 8

/**
 * NotificationBell, üst çubuktaki bildirim zilidir: okunmamış sayısı, son bildirimler ve
 * hepsini okundu sayma. Canlı akıştan gelen bildirim listeyi ve sayıyı tazeler, kısa bir uyarı
 * (toast) olarak da görünür.
 */
export function NotificationBell() {
  const { t, i18n } = useTranslation()
  const en = i18n.language.startsWith('en')
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const { data: unread } = useGetMyUnreadNotificationCountQuery()
  const { data: recent } = useListMyNotificationsQuery({ limit: recentLimit }, { skip: !open })
  const [markRead] = useMarkMyNotificationReadMutation()
  const [markAllRead, markingAll] = useMarkAllMyNotificationsReadMutation()

  useNotificationStream({
    onNotification: (n) => {
      dispatch(agoraApi.util.invalidateTags(['Bildirimler']))
      const { title, body } = notificationText(n, en)
      toast(title, {
        description: body,
        action: n.link
          ? { label: t('notifications.open'), onClick: () => void navigate(n.link!) }
          : undefined,
      })
    },
    // Yeni ileti: konuşma listesi, açık konuşma ve okunmamış sayısı tazelenir.
    onMessage: () => dispatch(agoraApi.util.invalidateTags(['Mesajlar'])),
  })

  const count = unread?.count ?? 0
  const select = (n: Notification) => {
    if (!n.read_at) {
      void markRead({ id: n.id })
    }
    if (n.link) {
      void navigate(n.link)
    }
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="relative"
          aria-label={count > 0 ? t('notifications.bellUnread', { count }) : t('notifications.title')}
        >
          <BellIcon />
          {count > 0 && (
            <span
              aria-hidden
              className="absolute -top-0.5 -right-0.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-gradient-to-br from-rose-500 to-pink-600 px-1 text-[11px] font-bold text-white tabular-nums ring-2 ring-background"
            >
              {count > 99 ? '99+' : count}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[min(24rem,calc(100vw-2rem))] p-0">
        <div className="flex items-center gap-2 px-4 py-3">
          <p className="flex-1 font-bold">{t('notifications.title')}</p>
          {count > 0 && (
            <Button
              variant="ghost"
              size="sm"
              disabled={markingAll.isLoading}
              onClick={() => void markAllRead()}
            >
              <CheckCheckIcon />
              {t('notifications.markAllRead')}
            </Button>
          )}
        </div>
        <DropdownMenuSeparator className="my-0" />
        <div className="max-h-[min(28rem,70vh)] overflow-y-auto p-1">
          {!recent ? (
            <div className="space-y-2 p-2">
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
            </div>
          ) : recent.items.length === 0 ? (
            <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t('notifications.empty')}</p>
          ) : (
            recent.items.map((n) => {
              const meta = categoryMeta[n.category]
              const { title, body } = notificationText(n, en)
              return (
                <DropdownMenuItem
                  key={n.id}
                  onSelect={() => select(n)}
                  className="flex items-start gap-3 rounded-lg px-3 py-2.5"
                >
                  <IconChip icon={meta.icon} tone={meta.tone} size="sm" className="mt-0.5" />
                  <span className="min-w-0 flex-1">
                    <span className={cn('block text-sm leading-snug', !n.read_at && 'font-bold')}>
                      {title}
                    </span>
                    {body && <span className="line-clamp-2 text-xs text-muted-foreground">{body}</span>}
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {formatRelative(n.created_at)}
                    </span>
                  </span>
                  {!n.read_at && (
                    <span
                      className="mt-1.5 size-2 shrink-0 rounded-full bg-rose-500"
                      aria-label={t('notifications.unread')}
                    />
                  )}
                </DropdownMenuItem>
              )
            })
          )}
        </div>
        <DropdownMenuSeparator className="my-0" />
        <DropdownMenuItem asChild className="justify-center rounded-none py-2.5 font-semibold text-primary">
          <Link to="/bildirimler">
            {t('notifications.seeAll')}
            <ArrowRightIcon />
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
