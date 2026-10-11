import { CalendarPlusIcon, CalendarSearchIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { TermSelect } from '@/features/calendar/TermSelect'
import { useTermParam } from '@/features/calendar/useTermParam'
import { RegistrationStatusBadge } from '@/features/registration/badges'
import { WeeklyPreview } from '@/features/registration/WeeklyPreview'
import { navTheme } from '@/features/shell/nav'
import {
  useListBuildingsQuery,
  useListClassroomsQuery,
  useListFacultiesQuery,
  useGetRegistrationQuery,
  useListFacultyDepartmentsQuery,
  useListMyProgramsQuery,
  useTermScheduleQuery,
  type ScheduleEntry,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
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

type View = 'benim' | 'bolum' | 'derslik' | 'hoca'

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
  // Öğrenci varsayılan olarak kendi ders programını görür (adreste başka bir seçim yoksa).
  const student = has('registration:manage_own')
  const chosen = ['birim', 'bolum', 'bina', 'derslik', 'hoca'].some((k) => params.has(k))
  const defaultView: View = student && !chosen ? 'benim' : 'bolum'
  const view = (params.get('gorunum') ?? defaultView) as View
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
      <PageHeader
        {...navTheme('/program')}
        title={t('schedule.title')}
        description={t('schedule.description')}
      />
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center">
        {view !== 'benim' && <TermSelect value={termId} onChange={setTermId} />}
        <Tabs value={view} onValueChange={(v) => set({ gorunum: v === defaultView ? '' : v, duzen: '' })}>
          <TabsList aria-label={t('schedule.view')}>
            {student && <TabsTrigger value="benim">{t('schedule.views.mine')}</TabsTrigger>}
            <TabsTrigger value="bolum">{t('schedule.views.department')}</TabsTrigger>
            <TabsTrigger value="derslik">{t('schedule.views.classroom')}</TabsTrigger>
            {has('section:manage') && (
              <TabsTrigger value="hoca">{t('schedule.views.instructor')}</TabsTrigger>
            )}
          </TabsList>
        </Tabs>
        {view !== 'benim' && (
          <Tabs value={layout} onValueChange={(v) => set({ duzen: v })} className="lg:ml-auto">
            <TabsList aria-label={t('schedule.layout')}>
              <TabsTrigger value="izgara">{t('schedule.layouts.grid')}</TabsTrigger>
              <TabsTrigger value="liste">{t('schedule.layouts.list')}</TabsTrigger>
            </TabsList>
          </Tabs>
        )}
      </div>
      {view === 'benim' ? (
        <MySchedule />
      ) : (
        <>
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
                  <Select
                    value={classroomId}
                    onValueChange={(v) => set({ derslik: v })}
                    disabled={!buildingId}
                  >
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
            <EmptyState icon={CalendarSearchIcon} tone="blue">
              {t(`schedule.choose.${view}`)}
            </EmptyState>
          ) : (
            <Schedule termId={termId} filter={filter} detail={detail} list={layout === 'liste'} />
          )}
        </>
      )}
    </>
  )
}

/** MySchedule, öğrencinin bu dönemki ders kaydındaki (sepetindeki) derslerin programıdır. */
function MySchedule() {
  const { t } = useTranslation()
  const programs = useListMyProgramsQuery()
  const program = programs.data?.items[0]
  const { data: r, error, refetch } = useGetRegistrationQuery({ id: program?.id ?? '' }, { skip: !program })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (programs.data && !program) {
    return <EmptyState>{t('registration.noPrograms')}</EmptyState>
  }
  if (!r) {
    return <Skeleton className="h-96" />
  }
  if (r.items.length === 0) {
    return (
      <EmptyState
        icon={CalendarPlusIcon}
        tone="violet"
        action={
          <Button asChild>
            <Link to="/ders-secimi">{t('schedule.mine.goRegistration')}</Link>
          </Button>
        }
      >
        {t('schedule.mine.empty', { term: r.term.code })}
      </EmptyState>
    )
  }
  return (
    <Card>
      <CardContent className="space-y-4">
        <p className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <RegistrationStatusBadge status={r.status} />
          {t('schedule.mine.hint', { term: r.term.code, count: r.items.length })}
        </p>
        <WeeklyPreview sections={r.items.map((it) => it.section)} />
      </CardContent>
    </Card>
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
