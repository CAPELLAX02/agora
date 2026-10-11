import { CheckIcon, MinusIcon, PlusIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  useApproveAddDropMutation,
  useListAddDropQueueQuery,
  useRejectAddDropMutation,
  type AddDropRequest,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { AddDropStatusBadge } from '../badges'
import { SectionSchedule } from '../SectionInfo'
import { newKey, useRun } from '../shared'
import { DecisionDialog } from './DecisionDialog'

/**
 * AddDropQueuePage, kararı beklenen ekle-bırak talepleridir: danışman için danışmanlığındaki
 * öğrencilerin danışman aşamasındaki talepleri, bölüm başkanı için bölümünün bölüm başkanı
 * aşamasındaki talepleri. Onay talebi bir sonraki aşamaya iletir ya da uygular.
 */
export function AddDropQueuePage() {
  const { t } = useTranslation()
  const { data, error, refetch } = useListAddDropQueueQuery({})
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-96" />
  }
  return (
    <>
      <PageHeader
        title={t('registration.addDropQueue.title')}
        description={t('registration.addDropQueue.description')}
      />
      {data.items.length === 0 ? (
        <EmptyState>{t('registration.addDropQueue.empty')}</EmptyState>
      ) : (
        <ul className="space-y-4">
          {data.items.map((q) => (
            <QueueItem key={q.id} request={q} />
          ))}
        </ul>
      )}
    </>
  )
}

function QueueItem({ request: q }: { request: AddDropRequest }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const run = useRun()
  const [approve] = useApproveAddDropMutation()
  const [reject] = useRejectAddDropMutation()
  const [decision, setDecision] = useState<'approve' | 'reject' | null>(null)
  const name = `${q.student.first_name} ${q.student.last_name}`
  return (
    <li>
      <Card role="region" aria-label={t('registration.addDropQueue.request', { name })}>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center justify-between gap-2 text-base">
            <span className="flex flex-wrap items-center gap-2">
              {name}
              <span className="font-mono text-xs font-normal text-muted-foreground">
                {q.student.student_no} · {q.student.program_code}
              </span>
              <AddDropStatusBadge status={q.status} />
              {q.stage && <Badge variant="outline">{t(`registration.addDrop.stage.${q.stage}`)}</Badge>}
            </span>
            <span className="flex gap-2">
              {q.actions.can_approve && (
                <Button size="sm" onClick={() => setDecision('approve')}>
                  <CheckIcon />
                  {t('registration.advisor.approve')}
                </Button>
              )}
              {q.actions.can_reject && (
                <Button size="sm" variant="outline" onClick={() => setDecision('reject')}>
                  <XIcon />
                  {t('registration.advisor.reject')}
                </Button>
              )}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <ul className="space-y-1.5">
            {q.items.map((it) => (
              <li key={it.id} className="flex flex-wrap items-center gap-2">
                {it.action === 'ADD' ? (
                  <PlusIcon className="size-4 text-success" aria-label={t('registration.addDrop.add')} />
                ) : (
                  <MinusIcon
                    className="size-4 text-destructive"
                    aria-label={t('registration.addDrop.drop')}
                  />
                )}
                <span className="font-medium">{it.section.label}</span>
                <span>{localized(it.section.course)}</span>
                <span className="text-xs text-muted-foreground">
                  · {t('registration.ects', { ects: it.ects })} · <SectionSchedule section={it.section} />
                </span>
              </li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            {t('registration.addDropQueue.meta', { time: formatDateTime(q.created_at), delta: q.ects_delta })}
          </p>
          {q.student_note && <p className="rounded-md bg-muted/40 px-3 py-2 text-sm">{q.student_note}</p>}
          {q.advisor && (
            <p className="text-xs text-muted-foreground">
              {t('registration.addDrop.advisorDecision', {
                by: q.advisor.by ?? '',
                time: formatDateTime(q.advisor.at),
              })}
              {q.advisor.note && ` — ${q.advisor.note}`}
            </p>
          )}
          <Link
            to={`/danismanlik/kayitlar/${q.registration_id}`}
            className="text-xs text-primary hover:underline"
          >
            {t('registration.addDropQueue.openRegistration')}
          </Link>
        </CardContent>
      </Card>
      {decision && (
        <DecisionDialog
          title={t(`registration.addDropQueue.${decision}.title`)}
          description={t(`registration.addDropQueue.${decision}.description`)}
          confirm={t(`registration.advisor.${decision}`)}
          required={decision === 'reject'}
          destructive={decision === 'reject'}
          onClose={() => setDecision(null)}
          onConfirm={(note) => {
            const args = { id: q.id, 'If-Match': ifMatch(q.version), 'Idempotency-Key': newKey() }
            const pending =
              decision === 'approve'
                ? approve({ ...args, body: note ? { note } : {} })
                : reject({ ...args, body: { note } })
            return run(pending, () => toast.success(t(`registration.addDropQueue.${decision}.done`)))
          }}
        />
      )}
    </li>
  )
}
