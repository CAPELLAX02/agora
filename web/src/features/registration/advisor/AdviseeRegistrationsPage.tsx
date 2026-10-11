import { CheckCheckIcon, ListChecksIcon, LockIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import {
  useBulkApproveRegistrationsMutation,
  useListAddDropQueueQuery,
  useListAdviseeRegistrationsQuery,
  type RegistrationStatus,
  type RegistrationSummary,
} from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { Checkbox } from '@/shared/ui/checkbox'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/shared/ui/tooltip'

import { RegistrationStatusBadge } from '../badges'
import { newKey, useRun } from '../shared'

const filters: (RegistrationStatus | 'ALL')[] = [
  'ALL',
  'SUBMITTED',
  'DRAFT',
  'APPROVED',
  'REJECTED',
  'NOT_STARTED',
]

/**
 * AdviseeRegistrationsPage, danışmanın öğrencilerinin dönemlik ders seçme durumlarıdır: onay
 * bekleyenler önce. Engeli olmayan, onay bekleyen kayıtlar seçilip toplu onaylanabilir (her
 * kayıt ayrı onaylanır; başarısız olanlar sonuçta listelenir).
 */
export function AdviseeRegistrationsPage() {
  const { t } = useTranslation()
  const run = useRun()
  const { data, error, refetch } = useListAdviseeRegistrationsQuery({})
  const { data: queue } = useListAddDropQueueQuery({})
  const [filter, setFilter] = useState<RegistrationStatus | 'ALL'>('ALL')
  const [selected, setSelected] = useState<string[]>([])
  const [confirming, setConfirming] = useState(false)
  const [bulk] = useBulkApproveRegistrationsMutation()

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-96" />
  }
  const count = (s: RegistrationStatus | 'ALL') =>
    s === 'ALL' ? data.items.length : data.items.filter((i) => i.status === s).length
  const rows = data.items.filter((i) => filter === 'ALL' || i.status === filter)
  const selectable = (i: RegistrationSummary) => i.status === 'SUBMITTED' && !i.blocked && i.registration_id
  const chosen = data.items.filter((i) => i.registration_id && selected.includes(i.registration_id))

  return (
    <>
      <PageHeader
        title={t('registration.advisor.title')}
        description={t('registration.advisor.description', { term: data.term.code })}
        actions={
          <>
            <Button asChild variant="outline">
              <Link to="/ekle-birak-onaylari">
                <ListChecksIcon />
                {t('registration.advisor.addDropQueue')}
                {queue && queue.items.length > 0 && <Badge variant="warning">{queue.items.length}</Badge>}
              </Link>
            </Button>
            <Button disabled={chosen.length === 0} onClick={() => setConfirming(true)}>
              <CheckCheckIcon />
              {t('registration.advisor.bulkApprove', { count: chosen.length })}
            </Button>
          </>
        }
      />
      <Tabs value={filter} onValueChange={(v) => setFilter(v as typeof filter)} className="mb-4">
        <TabsList aria-label={t('registration.advisor.filter')} className="flex-wrap">
          {filters.map((f) => (
            <TabsTrigger key={f} value={f}>
              {f === 'ALL' ? t('registration.advisor.all') : t(`registration.status.${f}`)} ({count(f)})
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <Card className="py-0">
        {rows.length === 0 ? (
          <EmptyState>{t('registration.advisor.empty')}</EmptyState>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <span className="sr-only">{t('registration.advisor.select')}</span>
                </TableHead>
                <TableHead>{t('registration.advisor.student')}</TableHead>
                <TableHead className="w-16 text-right">{t('registration.facts.semester')}</TableHead>
                <TableHead className="w-16 text-right">{t('registration.facts.gpa')}</TableHead>
                <TableHead className="w-24 text-right">{t('catalog.ects')}</TableHead>
                <TableHead className="w-32">{t('registration.advisor.status')}</TableHead>
                <TableHead className="w-40">{t('registration.advisor.submittedAt')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((i) => {
                const name = `${i.student.first_name} ${i.student.last_name}`
                return (
                  <TableRow key={i.student_program_id}>
                    <TableCell>
                      {selectable(i) && (
                        <Checkbox
                          aria-label={t('registration.advisor.selectStudent', { name })}
                          checked={selected.includes(i.registration_id!)}
                          onCheckedChange={(v) =>
                            setSelected(
                              v
                                ? [...selected, i.registration_id!]
                                : selected.filter((s) => s !== i.registration_id),
                            )
                          }
                        />
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        {i.registration_id ? (
                          <Link
                            to={`/danismanlik/kayitlar/${i.registration_id}`}
                            className="font-medium hover:underline"
                          >
                            {name}
                          </Link>
                        ) : (
                          <span className="font-medium">{name}</span>
                        )}
                        {i.blocked && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <LockIcon
                                className="size-3.5 text-destructive"
                                aria-label={t('registration.advisor.blocked')}
                              />
                            </TooltipTrigger>
                            <TooltipContent>{t('registration.advisor.blocked')}</TooltipContent>
                          </Tooltip>
                        )}
                      </div>
                      <p className="font-mono text-xs text-muted-foreground">{i.student.student_no}</p>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{i.semester}</TableCell>
                    <TableCell className="text-right tabular-nums">{i.gpa?.toFixed(2) ?? '—'}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {i.total_ects} <span className="text-xs text-muted-foreground">({i.item_count})</span>
                    </TableCell>
                    <TableCell>
                      <RegistrationStatusBadge status={i.status} />
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatDateTime(i.submitted_at)}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        )}
      </Card>
      {confirming && (
        <ConfirmDialog
          title={t('registration.advisor.bulkTitle')}
          description={t('registration.advisor.bulkDescription', { count: chosen.length })}
          confirm={t('registration.advisor.approve')}
          onClose={() => setConfirming(false)}
          onConfirm={async () => {
            await run(
              bulk({
                'Idempotency-Key': newKey(),
                body: { items: chosen.map((i) => ({ id: i.registration_id!, version: i.version })) },
              }),
              (res) => {
                if (res.failed === 0) {
                  toast.success(t('registration.advisor.bulkDone', { count: res.approved }))
                } else {
                  toast.warning(
                    t('registration.advisor.bulkPartial', { approved: res.approved, failed: res.failed }),
                    {
                      description: res.results
                        .filter((r) => !r.ok)
                        .map((r) => {
                          const s = data.items.find((i) => i.registration_id === r.id)
                          return `${s?.student.student_no ?? r.id}: ${r.message ?? ''}`
                        })
                        .join('\n'),
                    },
                  )
                }
                setSelected([])
                setConfirming(false)
              },
            )
          }}
        />
      )}
    </>
  )
}
