import { PlusIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { TermSelect } from '@/features/calendar/TermSelect'
import { useTermParam } from '@/features/calendar/useTermParam'
import {
  useGetDepartmentQuery,
  useListFacultiesQuery,
  useListFacultyDepartmentsQuery,
  useListOfferingsQuery,
  type ListOfferingsApiArg,
} from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Card, CardContent } from '@/shared/ui/card'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { OfferingStatusBadge } from './badges'
import { CreateOfferingDialog } from './CreateOfferingDialog'

/**
 * OfferingsPage, bir bölümün dönemde açtığı dersleri listeler; bölüm yönetimi buradan ders
 * açar ve ders açma ayrıntısına (şubeler, program, kontenjan) geçer. Bölüm başkanının
 * kendi bölümü varsayılan seçilir.
 */
export function OfferingsPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { grantsOf } = usePermissions()
  const [params, setParams] = useSearchParams()
  const [termId, setTermId] = useTermParam()
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const q = useDebounced(query.trim())

  const grants = grantsOf('offering:manage')
  const ownDepartment = grants.find((g) => g.scope_type === 'DEPARTMENT')?.scope_id ?? ''
  const ownFaculty = grants.find((g) => g.scope_type === 'FACULTY')?.scope_id ?? ''
  const own = useGetDepartmentQuery({ id: ownDepartment }, { skip: !ownDepartment || params.has('bolum') })
  const departmentId = params.get('bolum') ?? ownDepartment
  const facultyId = params.get('birim') ?? own.data?.faculty.id ?? ownFaculty

  const faculties = useListFacultiesQuery({})
  const departments = useListFacultyDepartmentsQuery({ id: facultyId }, { skip: !facultyId })
  const set = (changes: Record<string, string>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(changes)) {
      next.set(k, v)
    }
    setParams(next, { replace: true })
  }

  const filter: ListOfferingsApiArg = { id: termId, departmentId, limit: 100, ...(q && { q }) }

  return (
    <>
      <PageHeader
        title={t('offering.title')}
        description={t('offering.description')}
        actions={
          termId &&
          departmentId && (
            <Button onClick={() => setCreating(true)}>
              <PlusIcon />
              {t('offering.new')}
            </Button>
          )
        }
      />
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 border-b p-4 md:flex-row md:flex-wrap">
          <TermSelect value={termId} onChange={setTermId} />
          <Select value={facultyId} onValueChange={(v) => set({ birim: v, bolum: '' })}>
            <SelectTrigger className="md:w-56" aria-label={t('org.faculty')}>
              <SelectValue placeholder={t('org.faculty')} />
            </SelectTrigger>
            <SelectContent>
              {faculties.data?.items.map((f) => (
                <SelectItem key={f.id} value={f.id}>
                  {localized(f)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={departmentId}
            onValueChange={(v) => set({ birim: facultyId, bolum: v })}
            disabled={!facultyId}
          >
            <SelectTrigger className="md:w-56" aria-label={t('org.department')}>
              <SelectValue placeholder={t('org.department')} />
            </SelectTrigger>
            <SelectContent>
              {departments.data?.items.map((d) => (
                <SelectItem key={d.id} value={d.id}>
                  {localized(d)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="relative min-w-48 flex-1">
            <SearchIcon className="absolute top-2.5 left-3 size-4 text-muted-foreground" aria-hidden />
            <Input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('catalog.searchPlaceholder')}
              aria-label={t('common.search')}
              className="pl-9"
            />
          </div>
        </CardContent>
        {!termId || !departmentId ? (
          <EmptyState>{t('offering.chooseDepartment')}</EmptyState>
        ) : (
          <Offerings filter={filter} />
        )}
      </Card>
      {creating && (
        <CreateOfferingDialog
          termId={termId}
          departmentId={departmentId}
          onClose={() => setCreating(false)}
        />
      )}
    </>
  )
}

function Offerings({ filter }: { filter: ListOfferingsApiArg }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { data, error, refetch } = useListOfferingsQuery(filter)
  if (error) {
    return (
      <div className="p-4">
        <ErrorState error={error} onRetry={() => void refetch()} />
      </div>
    )
  }
  if (!data) {
    return (
      <div className="space-y-2 p-4">
        <Skeleton className="h-6" />
        <Skeleton className="h-6" />
        <Skeleton className="h-6" />
      </div>
    )
  }
  if (data.items.length === 0) {
    return <EmptyState>{t('offering.empty')}</EmptyState>
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-28">{t('catalog.code')}</TableHead>
          <TableHead>{t('catalog.name')}</TableHead>
          <TableHead>{t('offering.status')}</TableHead>
          <TableHead className="text-right">{t('offering.sections')}</TableHead>
          <TableHead className="hidden text-right sm:table-cell">{t('offering.enrolled')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {data.items.map((o) => (
          <TableRow key={o.id}>
            <TableCell className="font-mono text-xs">{o.course.code}</TableCell>
            <TableCell>
              <Link to={`/ders-acma/${o.id}`} className="font-medium hover:underline">
                {localized(o.course)}
              </Link>
            </TableCell>
            <TableCell>
              <OfferingStatusBadge status={o.status} />
            </TableCell>
            <TableCell className="text-right">{o.section_count}</TableCell>
            <TableCell className="hidden text-right tabular-nums sm:table-cell">
              {o.total_enrolled} / {o.total_capacity}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
