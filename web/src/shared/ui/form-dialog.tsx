import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from './alert'
import { Button } from './button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from './dialog'

/** FormDialog, başlık, hata ve gönder/vazgeç düğmeleri olan küçük bir form penceresidir. */
export function FormDialog({
  title,
  description,
  error,
  onClose,
  onSubmit,
  submit,
  children,
}: {
  title: string
  description?: string
  error: string | null
  onClose: () => void
  onSubmit: () => void
  submit: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent closeLabel={t('common.close')}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        <form
          className="space-y-4"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit()
          }}
        >
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {children}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            {submit}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
