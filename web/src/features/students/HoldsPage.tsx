import { PlusIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { formatDateTime } from '@/i18n/format'
import {
  useListHoldsQuery,
  useListStudentsQuery,
  type Hold,
  type HoldType,
  type ListHoldsApiArg,
  type StudentProgram,
} from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { HoldTypeBadge } from './badges'
import { PlaceHoldDialog, ReleaseHoldDialog } from './dialogs'

const types: HoldType[] = ['TUITION_DEBT', 'DISCIPLINE_SUSPENSION', 'MISSING_DOCUMENT', 'LIBRARY_DEBT']
const ALL = 'ALL'

/**
 * HoldsPage, yetkinin kapsadığı programlardaki kayıt engelleridir (mali işler, fakülte öğrenci
 * işleri): aktif ya da kaldırılmış, türe göre süzülür. Öğrenci aranıp engel konabilir, aktif
 * engel gerekçeyle kaldırılır.
 */
export function HoldsPage() {
  const { t } = useTranslation()
  const [active, setActive] = useState(true)
  const [type, setType] = useState<HoldType | typeof ALL>(ALL)
  const [query, setQuery] = useState('')
  const [placing, setPlacing] = useState(false)
  const [releasing, setReleasing] = useState<Hold | null>(null)
  const q = useDebounced(query.trim())
  const filter: ListHoldsApiArg = { active, limit: 100, ...(type !== ALL && { type }), ...(q && { q }) }
  const { data, error, refetch } = useListHoldsQuery(filter)

  return (
    <>
      <PageHeader
        title={t('students.holds.pageTitle')}
        description={t('students.holds.pageDescription')}
        actions={
          <Button onClick={() => setPlacing(true)}>
            <PlusIcon />
            {t('students.holds.place')}
          </Button>
        }
      />
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
        <Tabs value={active ? 'active' : 'released'} onValueChange={(v) => setActive(v === 'active')}>
          <TabsList>
            <TabsTrigger value="active">{t('students.holds.active')}</TabsTrigger>
            <TabsTrigger value="released">{t('students.holds.releasedTab')}</TabsTrigger>
          </TabsList>
        </Tabs>
        <Select value={type} onValueChange={(v) => setType(v as typeof type)}>
          <SelectTrigger className="sm:w-56" aria-label={t('students.holds.type')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t('common.all')}</SelectItem>
            {types.map((h) => (
              <SelectItem key={h} value={h}>
                {t(`students.holds.types.${h}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
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
      </div>
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !data ? (
        <Skeleton className="h-64" />
      ) : data.items.length === 0 ? (
        <EmptyState>{t('students.holds.empty')}</EmptyState>
      ) : (
        <Card className="py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('students.name')}</TableHead>
                <TableHead className="w-44">{t('students.holds.type')}</TableHead>
                <TableHead>{t('common.reason')}</TableHead>
                <TableHead className="w-48">{t('students.holds.date')}</TableHead>
                <TableHead className="w-28" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.map((h) => (
                <TableRow key={h.id}>
                  <TableCell>
                    <Link to={`/ogrenciler/${h.student.id}`} className="font-medium hover:underline">
                      {h.student.first_name} {h.student.last_name}
                    </Link>
                    <p className="font-mono text-xs text-muted-foreground">
                      {h.student.student_no} · {h.program.code}
                    </p>
                  </TableCell>
                  <TableCell>
                    <HoldTypeBadge type={h.hold_type} />
                  </TableCell>
                  <TableCell className="text-sm">
                    {h.reason}
                    {h.release_reason && <p className="text-xs text-muted-foreground">{h.release_reason}</p>}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDateTime(h.released_at ?? h.placed_at)}
                    <br />
                    {h.released_by ?? h.placed_by}
                  </TableCell>
                  <TableCell className="text-right">
                    {h.active && (
                      <Button size="sm" variant="outline" onClick={() => setReleasing(h)}>
                        {t('students.holds.release')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
      {placing && <PickStudent onClose={() => setPlacing(false)} />}
      {releasing && <ReleaseHoldDialog hold={releasing} onClose={() => setReleasing(null)} />}
    </>
  )
}

/** PickStudent, engel konacak öğrencinin program kaydını aratıp seçtirir, sonra engel formunu açar. */
function PickStudent({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [chosen, setChosen] = useState<StudentProgram | null>(null)
  const q = useDebounced(query.trim())
  const { data } = useListStudentsQuery({ q, limit: 10 }, { skip: q.length < 3 })
  if (chosen) {
    return <PlaceHoldDialog program={chosen} onClose={onClose} />
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent closeLabel={t('common.close')}>
        <DialogHeader>
          <DialogTitle>{t('students.holds.place')}</DialogTitle>
          <DialogDescription>{t('students.holds.pickStudent')}</DialogDescription>
        </DialogHeader>
        <Input
          type="search"
          autoFocus
          aria-label={t('students.search')}
          placeholder={t('students.search')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <ul className="max-h-72 space-y-1 overflow-y-auto">
          {data?.items.map((sp) => (
            <li key={sp.id}>
              <Button
                variant="ghost"
                className="h-auto w-full justify-start py-2 text-left"
                onClick={() => setChosen(sp)}
              >
                <span>
                  <span className="font-medium">
                    {sp.student.first_name} {sp.student.last_name}
                  </span>
                  <span className="block font-mono text-xs text-muted-foreground">
                    {sp.student.student_no} · {sp.program.code}
                  </span>
                </span>
              </Button>
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  )
}
