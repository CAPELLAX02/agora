import { FilePlus2Icon, Undo2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { navTheme } from '@/features/shell/nav'
import { useListGradeChangesQuery, useWithdrawGradeChangeMutation } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { ChangeCard } from './ChangeCard'
import { NewChangeDialog } from './NewChangeDialog'

/**
 * GradeChangesPage, öğretim elemanının not düzeltme talepleridir: harf notları ilan edildikten
 * sonra not sadece bu yolla değişir. Yeni talep verilen derslerin şubelerinden açılır, bekleyen
 * talep geri çekilebilir.
 */
export function GradeChangesPage() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const { data, error, refetch } = useListGradeChangesQuery({ view: 'mine' })
  const [creating, setCreating] = useState(false)
  const [withdrawing, setWithdrawing] = useState<string | null>(null)
  const [withdraw] = useWithdrawGradeChangeMutation()

  return (
    <>
      <PageHeader
        {...navTheme('/not-duzeltme')}
        title={t('gradeChanges.title')}
        description={t('gradeChanges.description')}
        actions={
          <Button onClick={() => setCreating(true)}>
            <FilePlus2Icon />
            {t('gradeChanges.new.open')}
          </Button>
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !data ? (
        <Skeleton className="h-64" />
      ) : data.items.length === 0 ? (
        <EmptyState {...navTheme('/not-duzeltme')}>{t('gradeChanges.empty')}</EmptyState>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {data.items.map((c) => (
            <ChangeCard
              key={c.id}
              change={c}
              actions={
                c.can_withdraw && (
                  <Button size="sm" variant="ghost" onClick={() => setWithdrawing(c.id)}>
                    <Undo2Icon />
                    {t('gradeChanges.withdraw')}
                  </Button>
                )
              }
            />
          ))}
        </div>
      )}
      {creating && <NewChangeDialog onClose={() => setCreating(false)} />}
      {withdrawing && (
        <ConfirmDialog
          title={t('gradeChanges.withdrawTitle')}
          description={t('gradeChanges.withdrawDescription')}
          confirm={t('gradeChanges.withdraw')}
          icon={Undo2Icon}
          tone="rose"
          onClose={() => setWithdrawing(null)}
          onConfirm={async () => {
            const res = await withdraw({ id: withdrawing })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return
            }
            toast.success(t('gradeChanges.withdrawn'))
            setWithdrawing(null)
          }}
        />
      )}
    </>
  )
}
