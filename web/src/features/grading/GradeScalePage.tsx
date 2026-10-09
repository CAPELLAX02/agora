import { CheckIcon, MinusIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { formatDate } from '@/i18n/format'
import {
  useListGradeScalesQuery,
  useListRegulationParametersQuery,
  type GradeItem,
  type RegulationParameter,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

/**
 * GradeScalePage, not ölçeğini (harf, katsayı, puan aralığı ve etkileri) ve bugün geçerli
 * yönetmelik parametrelerini gösterir. Kurallar veri olarak tutulur: değişince yeni ölçek ya
 * da yeni tarihli değer eklenir.
 */
export function GradeScalePage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const scales = useListGradeScalesQuery()
  const [active, setActive] = useState('')

  if (scales.error) {
    return <ErrorState error={scales.error} onRetry={() => void scales.refetch()} />
  }
  const scale = scales.data?.items.find((s) => s.id === active) ?? scales.data?.items[0]

  return (
    <>
      <PageHeader title={t('gradeScale.title')} description={t('gradeScale.description')} />
      <div className="space-y-6">
        {!scales.data ? (
          <Skeleton className="h-96" />
        ) : (
          scale && (
            <Card className="gap-0 py-0">
              <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3 border-b py-4">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2">
                    {localized(scale)}
                    {scale.is_default && <Badge variant="success">{t('gradeScale.default')}</Badge>}
                  </CardTitle>
                  <CardDescription>
                    {t('gradeScale.since', { year: scale.effective_from_year, code: scale.code })}
                  </CardDescription>
                </div>
                {scales.data.items.length > 1 && (
                  <Tabs value={scale.id} onValueChange={setActive}>
                    <TabsList>
                      {scales.data.items.map((s) => (
                        <TabsTrigger key={s.id} value={s.id}>
                          {s.code}
                        </TabsTrigger>
                      ))}
                    </TabsList>
                  </Tabs>
                )}
              </CardHeader>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('gradeScale.letter')}</TableHead>
                    <TableHead className="text-right">{t('gradeScale.coefficient')}</TableHead>
                    <TableHead>{t('gradeScale.range')}</TableHead>
                    <TableHead className="text-center">{t('gradeScale.passing')}</TableHead>
                    <TableHead className="text-center">{t('gradeScale.gpa')}</TableHead>
                    <TableHead className="text-center">{t('gradeScale.ects')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {scale.items.map((it) => (
                    <TableRow key={it.letter}>
                      <TableCell className="font-mono font-medium">{it.letter}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {it.coefficient?.toFixed(2) ?? '—'}
                      </TableCell>
                      <TableCell>
                        <Range item={it} />
                      </TableCell>
                      <TableCell className="text-center">
                        <Flag on={it.is_passing} label={t('gradeScale.passing')} />
                      </TableCell>
                      <TableCell className="text-center">
                        <Flag on={it.counts_in_gpa} label={t('gradeScale.gpa')} />
                      </TableCell>
                      <TableCell className="text-center">
                        <Flag on={it.earns_ects} label={t('gradeScale.ects')} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )
        )}
        <Regulations />
      </div>
    </>
  )
}

function Range({ item }: { item: GradeItem }) {
  const { t } = useTranslation()
  if (item.min_score !== null && item.max_score !== null) {
    return (
      <span className="tabular-nums">
        {item.min_score}–{item.max_score}
      </span>
    )
  }
  return (
    <span className="text-muted-foreground">
      {t(item.is_attendance_fail ? 'gradeScale.attendance' : 'gradeScale.noScore')}
    </span>
  )
}

function Flag({ on, label }: { on: boolean; label: string }) {
  const { t } = useTranslation()
  return on ? (
    <CheckIcon className="mx-auto size-4 text-success" aria-label={`${label}: ${t('common.yes')}`} />
  ) : (
    <MinusIcon className="mx-auto size-4 text-muted-foreground" aria-label={`${label}: ${t('common.no')}`} />
  )
}

function Regulations() {
  const { t } = useTranslation()
  const { data, error, refetch } = useListRegulationParametersQuery({})
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle>{t('gradeScale.regulations')}</CardTitle>
        <CardDescription>{t('gradeScale.regulationsHint')}</CardDescription>
      </CardHeader>
      {!data ? (
        <Skeleton className="m-4 h-40" />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('gradeScale.rule')}</TableHead>
              <TableHead>{t('gradeScale.value')}</TableHead>
              <TableHead className="hidden md:table-cell">{t('gradeScale.effective')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.items.map((p) => (
              <TableRow key={p.key}>
                <TableCell>
                  <p className="text-sm">{p.description_tr}</p>
                  <p className="font-mono text-xs text-muted-foreground">{p.key}</p>
                </TableCell>
                <TableCell>
                  <Value parameter={p} />
                </TableCell>
                <TableCell className="hidden text-sm text-muted-foreground md:table-cell">
                  {t('gradeScale.from', { date: formatDate(p.effective_from) })}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Card>
  )
}

/** Value, parametre değerini okunur gösterir: sayı olduğu gibi, nesne anahtar-değer, dizi satır satır. */
function Value({ parameter }: { parameter: RegulationParameter }) {
  const value = parameter.value as unknown
  if (Array.isArray(value)) {
    return (
      <ul className="space-y-0.5 font-mono text-xs">
        {value.map((v, i) => (
          <li key={i}>{describe(v)}</li>
        ))}
      </ul>
    )
  }
  return <span className="font-mono text-xs">{describe(value)}</span>
}

function describe(v: unknown): string {
  if (v !== null && typeof v === 'object') {
    return Object.entries(v)
      .map(([k, x]) => `${k}: ${String(x)}`)
      .join(', ')
  }
  return String(v)
}
