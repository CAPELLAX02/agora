import { AlertCircleIcon, InboxIcon, LockIcon, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { problemOf, type AnyError } from '@/shared/api/errors'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'

import { Alert, AlertDescription } from './alert'
import { Button } from './button'
import { IconChip } from './icon-chip'
import type { Tone } from './tone'

/** EmptyState, listenin boş olduğunu renkli bir ikonla, sakin bir dille gösterir. */
export function EmptyState({
  children,
  icon = InboxIcon,
  tone = 'violet',
  action,
  className,
}: {
  children: ReactNode
  icon?: LucideIcon
  tone?: Tone
  /** action, boş durumdan çıkışı sağlayan isteğe bağlı bir düğmedir. */
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center gap-3 px-4 py-12 text-center text-sm text-muted-foreground',
        className,
      )}
    >
      <IconChip icon={icon} tone={tone} size="xl" className="mb-1" />
      <div className="max-w-sm leading-relaxed">{children}</div>
      {action}
    </div>
  )
}

/** ErrorState, bir isteğin hatasını ve (varsa) tekrar deneme düğmesini gösterir. */
export function ErrorState({ error, onRetry }: { error: AnyError; onRetry?: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const requestId = problemOf(error)?.request_id
  return (
    <Alert variant="destructive">
      <AlertCircleIcon />
      <AlertDescription>
        <p>{message(error)}</p>
        {requestId && (
          <p className="font-mono text-xs opacity-70">{t('errors.requestId', { id: requestId })}</p>
        )}
        {onRetry && (
          <Button variant="outline" size="sm" className="mt-2" onClick={onRetry}>
            {t('common.retry')}
          </Button>
        )}
      </AlertDescription>
    </Alert>
  )
}

/** ForbiddenState, sayfanın kullanıcının yetkisi dışında olduğunu gösterir. */
export function ForbiddenState() {
  const { t } = useTranslation()
  return (
    <EmptyState icon={LockIcon} tone="amber">
      <span role="alert">{t('errors.forbidden')}</span>
    </EmptyState>
  )
}
