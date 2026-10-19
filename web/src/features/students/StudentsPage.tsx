import { SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { navTheme } from '@/features/shell/nav'
import { useListStudentsQuery, type EnrollmentStatus, type ListStudentsApiArg } from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Avatar } from '@/shared/ui/avatar'
import { Gpa } from '@/shared/ui/gpa'

import { EnrollmentStatusBadge } from './badges'

const statuses: EnrollmentStatus[] = [
  'ACTIVE',
  'PREP',
  'FROZEN',
  'SUSPENDED',
  'GRADUATED',
  'WITHDRAWN',
  'DISMISSED',
]
const ALL = 'ALL'

/** StudentsPage, yetkinin kapsadığı programların öğrenci kayıtlarıdır: numara ya da adla arama, duruma göre süzme. */
export function StudentsPage() {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<EnrollmentStatus | typeof ALL>(ALL)
  const q = useDebounced(query.trim())
  const filter: ListStudentsApiArg = { limit: 50, ...(q && { q }), ...(status !== ALL && { status }) }
  const key = JSON.stringify(filter)
  const [pages, setPages] = useState<{ key: string; cursors: string[] }>({ key, cursors: [''] })
  const cursors = pages.key === key ? pages.cursors : ['']

  return (
    <>
      <PageHeader
        {...navTheme('/ogrenciler')}
        title={t('students.title')}
        description={t('students.description')}
      />
      <div className="mb-4 flex flex-col gap-3 sm:flex-row">
        <div className="relative flex-1">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            className="pl-9"
            placeholder={t('students.search')}
            aria-label={t('students.search')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <Select value={status} onValueChange={(v) => setStatus(v as typeof status)}>
          <SelectTrigger className="sm:w-48" aria-label={t('students.statusFilter')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t('common.all')}</SelectItem>
            {statuses.map((s) => (
              <SelectItem key={s} value={s}>
                {t(`students.status.${s}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Card className="py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">{t('students.no')}</TableHead>
              <TableHead>{t('students.name')}</TableHead>
              <TableHead>{t('students.program')}</TableHead>
              <TableHead className="w-20 text-right">{t('registration.facts.semester')}</TableHead>
              <TableHead className="w-20 text-right">{t('registration.facts.gpa')}</TableHead>
              <TableHead className="w-32">{t('students.state')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cursors.map((cursor, i) => (
              <Rows
                key={cursor || 'first'}
                filter={cursor ? { ...filter, cursor } : filter}
                last={i === cursors.length - 1}
                onMore={(next) => setPages({ key, cursors: [...cursors, next] })}
              />
            ))}
          </TableBody>
        </Table>
      </Card>
    </>
  )
}

function Rows({
  filter,
  last,
  onMore,
}: {
  filter: ListStudentsApiArg
  last: boolean
  onMore: (cursor: string) => void
}) {
  const { t } = useTranslation()
  const { data, error, refetch } = useListStudentsQuery(filter)
  if (error) {
    return (
      <TableRow>
        <TableCell colSpan={6}>
          <ErrorState error={error} onRetry={() => void refetch()} />
        </TableCell>
      </TableRow>
    )
  }
  if (!data) {
    return (
      <TableRow>
        <TableCell colSpan={6}>
          <Skeleton className="h-24" />
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {data.items.length === 0 && !filter.cursor && (
        <TableRow>
          <TableCell colSpan={6}>
            <EmptyState>{t('students.empty')}</EmptyState>
          </TableCell>
        </TableRow>
      )}
      {data.items.map((sp) => (
        <TableRow key={sp.id}>
          <TableCell className="font-mono text-xs">{sp.student.student_no}</TableCell>
          <TableCell>
            <span className="flex items-center gap-2.5">
              <Avatar firstName={sp.student.first_name} lastName={sp.student.last_name} />
              <Link
                to={`/ogrenciler/${sp.student.id}`}
                className="font-semibold hover:text-primary hover:underline"
              >
                {sp.student.first_name} {sp.student.last_name}
              </Link>
            </span>
          </TableCell>
          <TableCell className="min-w-44 text-sm whitespace-normal">{sp.program.name_tr}</TableCell>
          <TableCell className="text-right tabular-nums">{sp.current_semester}</TableCell>
          <TableCell className="text-right">
            <Gpa value={sp.gpa} />
          </TableCell>
          <TableCell>
            <EnrollmentStatusBadge status={sp.status} />
          </TableCell>
        </TableRow>
      ))}
      {last && data.next_cursor && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={6} className="text-center">
            <Button variant="outline" size="sm" onClick={() => onMore(data.next_cursor!)}>
              {t('common.loadMore')}
            </Button>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
