import { CircleCheckBigIcon, Undo2Icon, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/shared/ui/button'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Label } from '@/shared/ui/label'
import { Textarea } from '@/shared/ui/textarea'
import type { Tone } from '@/shared/ui/tone'

/**
 * DecisionDialog, danışman (ya da bölüm başkanı, öğrenci işleri) kararını notla alır. Red ve
 * geri almada gerekçe zorunludur (en az 5 karakter); onayda not isteğe bağlıdır ama öğrenci
 * işlerinin pencere dışı müdahalesinde sunucu gerekçe ister.
 */
export function DecisionDialog({
  title,
  description,
  confirm,
  required,
  destructive = false,
  icon,
  tone,
  onClose,
  onConfirm,
}: {
  title: string
  description: string
  confirm: string
  required: boolean
  destructive?: boolean
  /** icon, başlığın yanındaki ikondur; verilmezse onayda onay, reddederken geri gönderme ikonu. */
  icon?: LucideIcon
  tone?: Tone
  onClose: () => void
  onConfirm: (note: string) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  return (
    <FormDialog
      title={title}
      description={description}
      icon={icon ?? (destructive ? Undo2Icon : CircleCheckBigIcon)}
      tone={tone ?? (destructive ? 'rose' : 'emerald')}
      error={error}
      onClose={onClose}
      onSubmit={() => {
        if (required && note.trim().length < 5) {
          setError(t('registration.decision.noteRequired'))
          return
        }
        setBusy(true)
        void onConfirm(note.trim()).then((ok) => {
          setBusy(false)
          if (ok) {
            onClose()
          }
        })
      }}
      submit={
        <Button type="submit" variant={destructive ? 'destructive' : 'default'} loading={busy}>
          {confirm}
        </Button>
      }
    >
      <div className="space-y-2">
        <Label htmlFor="decision-note">
          {required ? t('registration.decision.reason') : t('registration.decision.noteOptional')}
        </Label>
        <Textarea
          id="decision-note"
          value={note}
          maxLength={1000}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>
    </FormDialog>
  )
}
