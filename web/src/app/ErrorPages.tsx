import { CompassIcon, TriangleAlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useRouteError } from 'react-router'

import { Button } from '@/shared/ui/button'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className="grid min-h-[60svh] place-items-center p-6 text-center">
      <div className="space-y-4">
        <CompassIcon className="mx-auto size-10 text-muted-foreground" aria-hidden />
        <h1 className="text-2xl font-semibold">{t('errors.pageNotFound.title')}</h1>
        <p className="text-muted-foreground">{t('errors.pageNotFound.description')}</p>
        <Button asChild>
          <Link to="/">{t('errors.pageNotFound.home')}</Link>
        </Button>
      </div>
    </div>
  )
}

/** CrashPage, bir sayfa beklenmeyen bir hatayla çökünce gösterilir (router errorElement). */
export function CrashPage() {
  const { t } = useTranslation()
  const error = useRouteError()
  if (import.meta.env.DEV) {
    console.error(error)
  }
  return (
    <div className="grid min-h-svh place-items-center p-6 text-center">
      <div className="space-y-4">
        <TriangleAlertIcon className="mx-auto size-10 text-destructive" aria-hidden />
        <h1 className="text-2xl font-semibold">{t('errors.crash.title')}</h1>
        <p className="text-muted-foreground">{t('errors.crash.description')}</p>
        <Button onClick={() => window.location.reload()}>{t('errors.crash.reload')}</Button>
      </div>
    </div>
  )
}
