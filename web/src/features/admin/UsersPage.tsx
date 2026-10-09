import { PlusIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { formatDateTime } from '@/i18n/format'
import { useListUsersQuery, type ListUsersApiArg, type UserStatus } from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent } from '@/shared/ui/card'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { StatusBadge } from '@/shared/ui/status-badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { CreateUserDialog } from './CreateUserDialog'

const statuses: UserStatus[] = ['ACTIVE', 'PENDING', 'SUSPENDED', 'DISABLED']
const kinds = ['STUDENT', 'STAFF'] as const
const ALL = 'ALL'

/** UsersPage, hesapların aranabilir ve süzülebilir listesidir. Süzgeçler adreste tutulur. */
export function UsersPage() {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const [params, setParams] = useSearchParams()
  const [query, setQuery] = useState(params.get('q') ?? '')
  const [creating, setCreating] = useState(false)
  const q = useDebounced(query.trim())
  const status = (params.get('status') ?? '') as UserStatus | ''
  const kind = (params.get('kind') ?? '') as (typeof kinds)[number] | ''

  const filter: ListUsersApiArg = {
    limit: 25,
    ...(q && { q }),
    ...(status && { status }),
    ...(kind && { kind }),
  }
  const filterKey = JSON.stringify(filter)
  const [pages, setPages] = useState<{ key: string; cursors: string[] }>({ key: filterKey, cursors: [''] })
  // Süzgeç değişince liste baştan başlar.
  const cursors = pages.key === filterKey ? pages.cursors : ['']

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    if (value && value !== ALL) {
      next.set(key, value)
    } else {
      next.delete(key)
    }
    setParams(next, { replace: true })
  }

  return (
    <>
      <PageHeader
        title={t('users.title')}
        description={t('users.description')}
        actions={
          has('user:manage') && (
            <Button onClick={() => setCreating(true)}>
              <PlusIcon />
              {t('users.new')}
            </Button>
          )
        }
      />
      <Card className="gap-0 py-0">
        <CardContent className="flex flex-col gap-3 border-b p-4 sm:flex-row">
          <div className="relative flex-1">
            <SearchIcon className="absolute top-2.5 left-3 size-4 text-muted-foreground" aria-hidden />
            <Input
              type="search"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setParam('q', e.target.value.trim())
              }}
              placeholder={t('users.searchPlaceholder')}
              aria-label={t('common.search')}
              className="pl-9"
            />
          </div>
          <Select value={status || ALL} onValueChange={(v) => setParam('status', v)}>
            <SelectTrigger className="sm:w-48" aria-label={t('users.status')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('users.allStatuses')}</SelectItem>
              {statuses.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`status.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={kind || ALL} onValueChange={(v) => setParam('kind', v)}>
            <SelectTrigger className="sm:w-40" aria-label={t('users.kind')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('users.allKinds')}</SelectItem>
              {kinds.map((k) => (
                <SelectItem key={k} value={k}>
                  {t(`users.kinds.${k}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('users.name')}</TableHead>
              <TableHead>{t('users.username')}</TableHead>
              <TableHead className="hidden md:table-cell">{t('users.email')}</TableHead>
              <TableHead>{t('users.kind')}</TableHead>
              <TableHead>{t('users.status')}</TableHead>
              <TableHead className="hidden lg:table-cell">{t('users.lastLogin')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cursors.map((cursor, i) => (
              <UserRows
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
      {creating && <CreateUserDialog onClose={() => setCreating(false)} />}
    </>
  )
}

function UserRows({
  filter,
  cursor,
  last,
  onMore,
}: {
  filter: ListUsersApiArg
  cursor: string
  last: boolean
  onMore: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data, error, refetch, isFetching } = useListUsersQuery({ ...filter, ...(cursor && { cursor }) })
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
        {Array.from({ length: 5 }, (_, i) => (
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
          <EmptyState>{t('users.empty')}</EmptyState>
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {data.items.map((u) => (
        <TableRow key={u.id}>
          <TableCell>
            <Link to={`/yonetim/kullanicilar/${u.id}`} className="font-medium hover:underline">
              {u.first_name} {u.last_name}
            </Link>
          </TableCell>
          <TableCell className="font-mono text-xs">{u.username}</TableCell>
          <TableCell className="hidden text-muted-foreground md:table-cell">{u.email}</TableCell>
          <TableCell>
            {u.kind ? <Badge variant="outline">{t(`users.kinds.${u.kind}`)}</Badge> : '—'}
          </TableCell>
          <TableCell>
            <StatusBadge status={u.status} />
          </TableCell>
          <TableCell className="hidden text-muted-foreground lg:table-cell">
            {u.last_login_at ? formatDateTime(u.last_login_at) : t('users.never')}
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
