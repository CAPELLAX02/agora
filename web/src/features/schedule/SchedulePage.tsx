import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { TermSelect } from '@/features/calendar/TermSelect'
import { useTermParam } from '@/features/calendar/useTermParam'
import {
  useListBuildingsQuery,
  useListClassroomsQuery,
  useListFacultiesQuery,
  useListFacultyDepartmentsQuery,
  useTermScheduleQuery,
  type ScheduleEntry,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Card, CardContent } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { InstructorSearch } from './InstructorSearch'
import { instructorNames, roomLabel } from './layout'
import { WeeklyAgenda } from './WeeklyAgenda'
import { WeeklyGrid } from './WeeklyGrid'

type View = 'bolum' | 'derslik' | 'hoca'

/**
 * SchedulePage, dönemin haftalık programını bir bölümün açtığı dersler, bir dersliğin
 * doluluğu ya da bir öğretim elemanının dersleri olarak gösterir. Seçimler adreste tutulur.
 */
export function SchedulePage() {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const localized = useLocalized()
  const [params, setParams] = useSearchParams()
  const [termId, setTermId] = useTermParam()
  const view = (params.get('gorunum') ?? 'bolum') as View
  // Bölümün bütün dersleri ızgarada sıkışır: bölüm görünümünde varsayılan liste.
  const layout = params.get('duzen') ?? (view === 'bolum' ? 'liste' : 'izgara')
  const set = (changes: Record<string, string>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(changes)) {
      if (v) {
        next.set(k, v)
      } else {
        next.delete(k)
      }
    }
    setParams(next, { replace: true })
  }

  const facultyId = params.get('birim') ?? ''
  const departmentId = params.get('bolum') ?? ''
  const buildingId = params.get('bina') ?? ''
  const classroomId = params.get('derslik') ?? ''
  const staffId = params.get('hoca') ?? ''
  const faculties = useListFacultiesQuery({}, { skip: view !== 'bolum' })
  const departments = useListFacultyDepartmentsQuery(
    { id: facultyId },
    { skip: view !== 'bolum' || !facultyId },
  )
  const buildings = useListBuildingsQuery({}, { skip: view !== 'derslik' })
  const classrooms = useListClassroomsQuery(
    { buildingId, limit: 100 },
    { skip: view !== 'derslik' || !buildingId },
  )

  const filter = view === 'bolum' ? { departmentId } : view === 'derslik' ? { classroomId } : { staffId }
  const target = view === 'bolum' ? departmentId : view === 'derslik' ? classroomId : staffId

  const detail = (e: ScheduleEntry) =>
    view === 'derslik'
      ? instructorNames(e)
      : view === 'hoca'
        ? (roomLabel(e) ?? t('schedule.online'))
        : [roomLabel(e), instructorNames(e)].filter(Boolean).join(' · ')

  return (
    <>
      <PageHeader title={t('schedule.title')} description={t('schedule.description')} />
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center">
        <TermSelect value={termId} onChange={setTermId} />
        <Tabs value={view} onValueChange={(v) => set({ gorunum: v === 'bolum' ? '' : v, duzen: '' })}>
          <TabsList aria-label={t('schedule.view')}>
            <TabsTrigger value="bolum">{t('schedule.views.department')}</TabsTrigger>
            <TabsTrigger value="derslik">{t('schedule.views.classroom')}</TabsTrigger>
            {has('section:manage') && (
              <TabsTrigger value="hoca">{t('schedule.views.instructor')}</TabsTrigger>
            )}
          </TabsList>
        </Tabs>
        <Tabs value={layout} onValueChange={(v) => set({ duzen: v })} className="lg:ml-auto">
          <TabsList aria-label={t('schedule.layout')}>
            <TabsTrigger value="izgara">{t('schedule.layouts.grid')}</TabsTrigger>
            <TabsTrigger value="liste">{t('schedule.layouts.list')}</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      <Card className="mb-6">
        <CardContent className="flex flex-col gap-3 sm:flex-row">
          {view === 'bolum' && (
            <>
              <Select value={facultyId} onValueChange={(v) => set({ birim: v, bolum: '' })}>
                <SelectTrigger className="sm:w-64" aria-label={t('org.faculty')}>
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
              <Select value={departmentId} onValueChange={(v) => set({ bolum: v })} disabled={!facultyId}>
                <SelectTrigger className="sm:w-64" aria-label={t('org.department')}>
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
            </>
          )}
          {view === 'derslik' && (
            <>
              <Select value={buildingId} onValueChange={(v) => set({ bina: v, derslik: '' })}>
                <SelectTrigger className="sm:w-64" aria-label={t('schedule.building')}>
                  <SelectValue placeholder={t('schedule.building')} />
                </SelectTrigger>
                <SelectContent>
                  {buildings.data?.items.map((b) => (
                    <SelectItem key={b.id} value={b.id}>
                      {b.code} · {b.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={classroomId} onValueChange={(v) => set({ derslik: v })} disabled={!buildingId}>
                <SelectTrigger className="sm:w-64" aria-label={t('schedule.classroom')}>
                  <SelectValue placeholder={t('schedule.classroom')} />
                </SelectTrigger>
                <SelectContent>
                  {classrooms.data?.items.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.code} · {c.name} ({t('schedule.capacity', { count: c.capacity })})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </>
          )}
          {view === 'hoca' && <InstructorSearch value={staffId} onChange={(id) => set({ hoca: id })} />}
        </CardContent>
      </Card>
      {!termId || !target ? (
        <EmptyState>{t(`schedule.choose.${view}`)}</EmptyState>
      ) : (
        <Schedule termId={termId} filter={filter} detail={detail} list={layout === 'liste'} />
      )}
    </>
  )
}

function Schedule({
  termId,
  filter,
  detail,
  list,
}: {
  termId: string
  filter: { departmentId?: string; classroomId?: string; staffId?: string }
  detail: (e: ScheduleEntry) => string
  list: boolean
}) {
  const { data, error, refetch } = useTermScheduleQuery({ id: termId, ...filter })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-96" />
  }
  return list ? <WeeklyAgenda entries={data.items} /> : <WeeklyGrid entries={data.items} detail={detail} />
}
