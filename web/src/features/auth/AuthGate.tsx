import { useEffect, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { refreshSession } from '@/shared/api/baseQuery'
import { BrandMark } from '@/shared/ui/brand'
import { Spinner } from '@/shared/ui/spinner'

import { restoreFailed } from './authSlice'

/**
 * AuthGate, açılışta oturumu geri yükler: access token bellekte tutulduğu için sayfa
 * yenilenince kaybolur, HttpOnly refresh çereziyle sessizce yenisi alınır. Sonuç belli
 * olana kadar uygulama yerine bir bekleme ekranı gösterilir.
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const dispatch = useAppDispatch()
  const status = useAppSelector((s) => s.auth.status)

  useEffect(() => {
    if (status !== 'restoring') {
      return
    }
    // StrictMode'da efekt iki kez çalışır: refreshSession aynı isteği paylaştırır.
    void refreshSession(dispatch, { retry: false }).then((result) => {
      if (result !== 'ok') {
        dispatch(restoreFailed())
      }
    })
  }, [status, dispatch])

  if (status === 'restoring') {
    return (
      <div className="grid min-h-svh place-items-center">
        <div className="flex flex-col items-center gap-4">
          <BrandMark className="size-12" />
          <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Spinner /> {t('auth.restoring')}
          </p>
        </div>
      </div>
    )
  }
  return children
}
