import { AlertCircleIcon, InboxIcon, LockIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { problemOf, type AnyError } from '@/shared/api/errors'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'

import { Alert, AlertDescription } from './alert'
import { Button } from './button'

/** EmptyState, listenin boş olduğunu gösterir. */
export function EmptyState({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        'flex flex-col items-center gap-2 py-10 text-center text-sm text-muted-foreground',
        className,
      )}
    >
      <InboxIcon className="size-8 opacity-40" aria-hidden />
      {children}
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
    <Alert variant="warning">
      <LockIcon />
      <AlertDescription>{t('errors.forbidden')}</AlertDescription>
    </Alert>
  )
}
