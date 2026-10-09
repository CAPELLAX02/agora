import { SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import {
  useListCoursesQuery,
  useListFacultiesQuery,
  useListFacultyDepartmentsQuery,
  type CourseKind,
  type ListCoursesApiArg,
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

import { CourseKindBadge } from './course'
import { hours } from './format'

const kinds: CourseKind[] = ['REGULAR', 'NON_CREDIT', 'INTERNSHIP', 'PROJECT', 'PREP', 'ACTIVITY']
const ALL = 'ALL'

/**
 * CoursesPage, ders kataloğunun aranabilir listesidir. Ders kodu ya da adıyla aranır,
 * bölüme ve ders türüne göre süzülür; süzgeçler adreste tutulur.
 */
export function CoursesPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [params, setParams] = useSearchParams()
  const [query, setQuery] = useState(params.get('q') ?? '')
  const q = useDebounced(query.trim())
  const facultyId = params.get('birim') ?? ''
  const departmentId = params.get('bolum') ?? ''
  const kind = (params.get('tur') ?? '') as CourseKind | ''
  const faculties = useListFacultiesQuery({})
  const departments = useListFacultyDepartmentsQuery({ id: facultyId }, { skip: !facultyId })

  const filter: ListCoursesApiArg = {
    limit: 50,
    ...(q && { q }),
    ...(departmentId && { departmentId }),
    ...(kind && { kind }),
  }
  const filterKey = JSON.stringify(filter)
  const [pages, setPages] = useState<{ key: string; cursors: string[] }>({ key: filterKey, cursors: [''] })
  const cursors = pages.key === filterKey ? pages.cursors : ['']

  const setParam = (changes: Record<string, string>) => {
    const next = new URLSearchParams(params)
    for (const [key, value] of Object.entries(changes)) {
      if (value && value !== ALL) {
        next.set(key, value)
      } else {
        next.delete(key)
      }
    }
    setParams(next, { replace: true })
  }

  return (
    <>
      <PageHeader title={t('catalog.title')} description={t('catalog.description')} />
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 border-b p-4 lg:flex-row">
          <div className="relative flex-1">
            <SearchIcon className="absolute top-2.5 left-3 size-4 text-muted-foreground" aria-hidden />
            <Input
              type="search"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setParam({ q: e.target.value.trim() })
              }}
              placeholder={t('catalog.searchPlaceholder')}
              aria-label={t('common.search')}
              className="pl-9"
            />
          </div>
          <Select value={facultyId || ALL} onValueChange={(v) => setParam({ birim: v, bolum: '' })}>
            <SelectTrigger className="lg:w-56" aria-label={t('catalog.faculty')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('catalog.allFaculties')}</SelectItem>
              {faculties.data?.items.map((f) => (
                <SelectItem key={f.id} value={f.id}>
                  {localized(f)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={departmentId || ALL}
            onValueChange={(v) => setParam({ bolum: v })}
            disabled={!facultyId}
          >
            <SelectTrigger className="lg:w-56" aria-label={t('catalog.department')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('catalog.allDepartments')}</SelectItem>
              {departments.data?.items.map((d) => (
                <SelectItem key={d.id} value={d.id}>
                  {localized(d)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={kind || ALL} onValueChange={(v) => setParam({ tur: v })}>
            <SelectTrigger className="lg:w-44" aria-label={t('catalog.kind')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('catalog.allKinds')}</SelectItem>
              {kinds.map((k) => (
                <SelectItem key={k} value={k}>
                  {t(`catalog.kinds.${k}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">{t('catalog.code')}</TableHead>
              <TableHead>{t('catalog.name')}</TableHead>
              <TableHead className="hidden sm:table-cell">{t('catalog.hours')}</TableHead>
              <TableHead className="hidden sm:table-cell">{t('catalog.credit')}</TableHead>
              <TableHead>{t('catalog.ects')}</TableHead>
              <TableHead className="hidden lg:table-cell">{t('catalog.owner')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cursors.map((cursor, i) => (
              <CourseRows
                key={`${filterKey}:${cursor}`}
                filter={filter}
                cursor={cursor}
                last={i === cursors.length - 1}
                onMore={(next) => setPages({ key: filterKey, cursors: [...cursors, next] })}
              />
            ))}
          </TableBody>
        </Table>
      </Card>
    </>
  )
}

function CourseRows({
  filter,
  cursor,
  last,
  onMore,
}: {
  filter: ListCoursesApiArg
  cursor: string
  last: boolean
  onMore: (next: string) => void
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { data, error, refetch, isFetching } = useListCoursesQuery({ ...filter, ...(cursor && { cursor }) })
  const span = 6

  if (error) {
    return (
      <TableRow>
        <TableCell colSpan={span}>
          <ErrorState error={error} onRetry={() => void refetch()} />
        </TableCell>
      </TableRow>
    )
  }
  if (!data) {
    return (
      <>
        {Array.from({ length: 6 }, (_, i) => (
          <TableRow key={i}>
            <TableCell colSpan={span}>
              <Skeleton className="h-5" />
            </TableCell>
          </TableRow>
        ))}
      </>
    )
  }
  if (data.items.length === 0 && cursor === '') {
    return (
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={span}>
          <EmptyState>{t('catalog.empty')}</EmptyState>
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {data.items.map((c) => (
        <TableRow key={c.id}>
          <TableCell className="font-mono text-xs">{c.code}</TableCell>
          <TableCell>
            <div className="flex flex-wrap items-center gap-2">
              <Link to={`/dersler/${c.id}`} className="font-medium hover:underline">
                {localized(c)}
              </Link>
              <CourseKindBadge kind={c.kind} />
            </div>
          </TableCell>
          <TableCell className="hidden font-mono text-xs sm:table-cell">{hours(c)}</TableCell>
          <TableCell className="hidden sm:table-cell">{c.national_credit}</TableCell>
          <TableCell>{c.ects}</TableCell>
          <TableCell className="hidden text-muted-foreground lg:table-cell">
            {c.owner_department ? localized(c.owner_department) : t('catalog.universityCommon')}
          </TableCell>
        </TableRow>
      ))}
      {last && data.next_cursor && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={span} className="text-center">
            <Button
              variant="ghost"
              size="sm"
              loading={isFetching}
              onClick={() => onMore(data.next_cursor ?? '')}
            >
              {t('common.loadMore')}
            </Button>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
