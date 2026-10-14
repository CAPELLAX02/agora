import { CircleCheckBigIcon, XCircleIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DecisionDialog } from '@/features/registration/advisor/DecisionDialog'
import { navTheme } from '@/features/shell/nav'
import {
  useApproveGradeChangeMutation,
  useListGradeChangesQuery,
  useRejectGradeChangeMutation,
  type GradeChangeRequest,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { ChangeCard } from './ChangeCard'

/**
 * ReviewPage, bölüm başkanı ve fakülte öğrenci işlerinin not düzeltme onay kuyruğudur: kapsamdaki
 * bekleyen talepler ve son kararlar. Onay notu hemen uygular (MFA'lı oturum); talep eden kendi
 * talebini onaylayamaz. Ret gerekçe ister.
 */
export function ReviewPage() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const pending = useListGradeChangesQuery({ view: 'review', status: 'PENDING' })
  const decided = useListGradeChangesQuery({ view: 'review', status: 'APPLIED,REJECTED' })
  const [deciding, setDeciding] = useState<{ change: GradeChangeRequest; approve: boolean } | null>(null)
  const [approve] = useApproveGradeChangeMutation()
  const [reject] = useRejectGradeChangeMutation()

  const error = pending.error ?? decided.error
  return (
    <>
      <PageHeader
        {...navTheme('/not-duzeltme-onaylari')}
        title={t('gradeChanges.review.title')}
        description={t('gradeChanges.review.description')}
      />
      {error ? (
        <ErrorState error={error} onRetry={() => void pending.refetch()} />
      ) : !pending.data || !decided.data ? (
        <Skeleton className="h-64" />
      ) : (
        <div className="space-y-8">
          <section aria-labelledby="pending-changes" className="space-y-3">
            <h2 id="pending-changes" className="text-lg font-bold">
              {t('gradeChanges.review.pending', { count: pending.data.items.length })}
            </h2>
            {pending.data.items.length === 0 ? (
              <EmptyState {...navTheme('/not-duzeltme-onaylari')}>
                {t('gradeChanges.review.empty')}
              </EmptyState>
            ) : (
              <div className="grid gap-4 lg:grid-cols-2">
                {pending.data.items.map((c) => (
                  <ChangeCard
                    key={c.id}
                    change={c}
                    actions={
                      c.can_approve && (
                        <>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setDeciding({ change: c, approve: false })}
                          >
                            <XCircleIcon />
                            {t('gradeChanges.review.reject')}
                          </Button>
                          <Button size="sm" onClick={() => setDeciding({ change: c, approve: true })}>
                            <CircleCheckBigIcon />
                            {t('gradeChanges.review.approve')}
                          </Button>
                        </>
                      )
                    }
                  />
                ))}
              </div>
            )}
          </section>
          {decided.data.items.length > 0 && (
            <section aria-labelledby="decided-changes" className="space-y-3">
              <h2 id="decided-changes" className="text-lg font-bold">
                {t('gradeChanges.review.decided')}
              </h2>
              <div className="grid gap-4 lg:grid-cols-2">
                {decided.data.items.slice(0, 10).map((c) => (
                  <ChangeCard key={c.id} change={c} />
                ))}
              </div>
            </section>
          )}
        </div>
      )}
      {deciding && (
        <DecisionDialog
          title={
            deciding.approve ? t('gradeChanges.review.approveTitle') : t('gradeChanges.review.rejectTitle')
          }
          description={
            deciding.approve
              ? t('gradeChanges.review.approveDescription')
              : t('gradeChanges.review.rejectDescription')
          }
          confirm={deciding.approve ? t('gradeChanges.review.approve') : t('gradeChanges.review.reject')}
          required={!deciding.approve}
          destructive={!deciding.approve}
          icon={deciding.approve ? CircleCheckBigIcon : XCircleIcon}
          onClose={() => setDeciding(null)}
          onConfirm={async (note) => {
            const res = deciding.approve
              ? await approve({ id: deciding.change.id, body: note ? { note } : {} })
              : await reject({ id: deciding.change.id, body: { note } })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return false
            }
            toast.success(
              deciding.approve ? t('gradeChanges.review.approved') : t('gradeChanges.review.rejected'),
            )
            return true
          }}
        />
      )}
    </>
  )
}
