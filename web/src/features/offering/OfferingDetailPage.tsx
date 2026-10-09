import { ArrowLeftIcon, PlusIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'

import { usePermissions } from '@/features/auth/usePermissions'
import { ifMatch } from '@/shared/api/etag'
import {
  useDeleteOfferingMutation,
  useGetOfferingQuery,
  useUpdateOfferingMutation,
  type OfferingDetail,
  type OfferingStatus,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { OfferingStatusBadge } from './badges'
import { SectionDialog } from './dialogs'
import { SectionCard } from './SectionCard'

/** Durumdan yapılabilecek geçişler (sunucudaki kurallarla aynı). */
const transitions: Record<OfferingStatus, OfferingStatus[]> = {
  PLANNED: ['OPEN', 'CANCELLED'],
  OPEN: ['CLOSED', 'PLANNED', 'CANCELLED'],
  CLOSED: [],
  CANCELLED: [],
}

/**
 * OfferingDetailPage, açılan bir dersin şubelerini, öğretim elemanlarını, program kontenjanlarını
 * ve haftalık programını yönetir. Derslik ya da öğretim elemanı çakışmasında sunucunun
 * çakışan dersi ve saati söyleyen mesajı gösterilir.
 */
export function OfferingDetailPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const localized = useLocalized()
  const { has } = usePermissions()
  const { data: offering, error, refetch } = useGetOfferingQuery({ id })
  const [adding, setAdding] = useState(false)

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!offering) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-96" />
        <Skeleton className="h-64" />
      </div>
    )
  }
  const editable = offering.status === 'PLANNED' || offering.status === 'OPEN'

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
        <Link to={`/ders-acma?donem=${offering.term.id}&bolum=${offering.department.id}`}>
          <ArrowLeftIcon />
          {t('offering.title')}
        </Link>
      </Button>
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            <span className="font-mono text-muted-foreground">{offering.course.code}</span>
            {localized(offering.course)}
            <OfferingStatusBadge status={offering.status} />
          </span>
        }
        description={t('offering.detail.subtitle', {
          term: offering.term.code,
          department: localized(offering.department),
          hours: `${offering.course.theory_hours}-${offering.course.practice_hours}`,
          ects: offering.course.ects,
        })}
        actions={
          <div className="flex flex-wrap gap-2">
            {has('offering:manage') && <StatusActions offering={offering} />}
            {editable && has('section:manage') && (
              <Button onClick={() => setAdding(true)}>
                <PlusIcon />
                {t('offering.section.add')}
              </Button>
            )}
          </div>
        }
      />
      {offering.sections.length === 0 ? (
        <EmptyState>{t('offering.detail.noSections')}</EmptyState>
      ) : (
        <div className="space-y-6">
          {offering.sections.map((s) => (
            <SectionCard key={s.id} section={s} editable={editable} />
          ))}
        </div>
      )}
      {adding && <SectionDialog offering={offering} onClose={() => setAdding(false)} />}
    </>
  )
}

function StatusActions({ offering }: { offering: OfferingDetail }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const message = useErrorMessage()
  const [update] = useUpdateOfferingMutation()
  const [remove] = useDeleteOfferingMutation()
  const [confirm, setConfirm] = useState<OfferingStatus | 'DELETE' | null>(null)

  const apply = async (status: OfferingStatus) => {
    const res = await update({
      id: offering.id,
      'If-Match': ifMatch(offering.version),
      offeringUpdateRequest: {
        status,
        ...(offering.external_ref && { external_ref: offering.external_ref }),
        ...(offering.note && { note: offering.note }),
      },
    })
    if (res.error) {
      toast.error(message(res.error))
      return
    }
    toast.success(t(`offering.transitions.done.${status}`))
    setConfirm(null)
  }

  return (
    <>
      {transitions[offering.status].map((s) => (
        <Button
          key={s}
          variant={s === 'OPEN' ? 'default' : 'outline'}
          onClick={() => (s === 'CANCELLED' ? setConfirm(s) : void apply(s))}
        >
          {t(`offering.transitions.to.${s}`)}
        </Button>
      ))}
      {offering.status === 'PLANNED' && (
        <Button variant="ghost" onClick={() => setConfirm('DELETE')}>
          {t('offering.transitions.delete')}
        </Button>
      )}
      {confirm === 'CANCELLED' && (
        <ConfirmDialog
          title={t('offering.transitions.cancelTitle')}
          description={t('offering.transitions.cancelDescription')}
          confirm={t('offering.transitions.to.CANCELLED')}
          destructive
          onClose={() => setConfirm(null)}
          onConfirm={() => apply('CANCELLED')}
        />
      )}
      {confirm === 'DELETE' && (
        <ConfirmDialog
          title={t('offering.transitions.deleteTitle')}
          description={t('offering.transitions.deleteDescription')}
          confirm={t('offering.transitions.delete')}
          destructive
          onClose={() => setConfirm(null)}
          onConfirm={async () => {
            const res = await remove({ id: offering.id })
            if (res.error) {
              toast.error(message(res.error))
              return
            }
            toast.success(t('offering.transitions.deleted'))
            void navigate(`/ders-acma?donem=${offering.term.id}&bolum=${offering.department.id}`)
          }}
        />
      )}
    </>
  )
}
