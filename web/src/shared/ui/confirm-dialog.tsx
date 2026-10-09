import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from './alert-dialog'
import { Button } from './button'

/**
 * ConfirmDialog, geri alınamayan bir işlem öncesi onay ister. Onay düğmesi işlem bitene
 * kadar bekler; işlem başarısız olursa pencere açık kalır (onConfirm kapatmaya karar verir).
 */
export function ConfirmDialog({
  title,
  description,
  confirm,
  destructive = false,
  onClose,
  onConfirm,
  children,
}: {
  title: string
  description: ReactNode
  confirm: string
  destructive?: boolean
  onClose: () => void
  onConfirm: () => Promise<void> | void
  children?: ReactNode
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  return (
    <AlertDialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>{t('common.cancel')}</AlertDialogCancel>
          <Button
            variant={destructive ? 'destructive' : 'default'}
            loading={busy}
            onClick={() => {
              setBusy(true)
              void Promise.resolve(onConfirm()).finally(() => setBusy(false))
            }}
          >
            {confirm}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
