import { CompassIcon, HomeIcon, RotateCwIcon, TriangleAlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useRouteError } from 'react-router'

import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className="grid min-h-[60svh] place-items-center p-6 text-center">
      <div className="flex max-w-md flex-col items-center gap-4">
        <p aria-hidden className="text-brand-gradient text-8xl font-extrabold tracking-tighter">
          404
        </p>
        <IconChip icon={CompassIcon} tone="violet" size="lg" className="-mt-2 rounded-full" />
        <h1 className="text-2xl font-extrabold tracking-tight">{t('errors.pageNotFound.title')}</h1>
        <p className="text-muted-foreground">{t('errors.pageNotFound.description')}</p>
        <Button asChild size="lg" variant="brand">
          <Link to="/">
            <HomeIcon />
            {t('errors.pageNotFound.home')}
          </Link>
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
    <div className="grid min-h-svh place-items-center app-glow p-6 text-center">
      <div className="flex max-w-md flex-col items-center gap-4">
        <IconChip icon={TriangleAlertIcon} tone="rose" size="xl" solid className="rounded-full" />
        <h1 className="text-2xl font-extrabold tracking-tight">{t('errors.crash.title')}</h1>
        <p className="text-muted-foreground">{t('errors.crash.description')}</p>
        <Button size="lg" onClick={() => window.location.reload()}>
          <RotateCwIcon />
          {t('errors.crash.reload')}
        </Button>
      </div>
    </div>
  )
}
