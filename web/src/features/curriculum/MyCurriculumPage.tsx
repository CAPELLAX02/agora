import { InfoIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useMyCurriculaQuery, type MyCurriculaApiResponse } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { YearRange } from './badges'
import { CurriculumView } from './CurriculumView'

type Enrollment = MyCurriculaApiResponse['items'][number]

/**
 * MyCurriculumPage, öğrencinin izlediği ders planıdır. Çift anadal ya da yandal
 * öğrencisinin her program kaydı için ayrı bir planı olur; program bağlamı sekmelerle seçilir.
 */
export function MyCurriculumPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { data, error, refetch } = useMyCurriculaQuery()
  const [active, setActive] = useState('')

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="h-96" />
      </div>
    )
  }

  const current = data.items.find((e) => e.student_program_id === active) ?? data.items[0]

  return (
    <>
      <PageHeader title={t('curriculum.my.title')} description={t('curriculum.my.description')} />
      {!current ? (
        <EmptyState>{t('curriculum.my.noPrograms')}</EmptyState>
      ) : (
        <div className="space-y-6">
          {data.items.length > 1 && (
            <Tabs value={current.student_program_id} onValueChange={setActive}>
              <TabsList aria-label={t('curriculum.my.programs')}>
                {data.items.map((e) => (
                  <TabsTrigger key={e.student_program_id} value={e.student_program_id}>
                    {localized(e.program)} · {t(`curriculum.my.kinds.${e.enrollment_kind}`)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}
          <Enrollment enrollment={current} />
        </div>
      )}
    </>
  )
}

function Enrollment({ enrollment: e }: { enrollment: Enrollment }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  if (!e.curriculum) {
    return (
      <Alert>
        <InfoIcon />
        <AlertDescription>
          {t('curriculum.my.notAssigned', { program: localized(e.program) })}
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold">
          {localized(e.program)} — {localized(e.curriculum)}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('curriculum.my.admission', { year: e.admission_year })} ·{' '}
          <YearRange from={e.curriculum.effective_from_year} to={e.curriculum.effective_to_year} />
        </p>
      </div>
      <CurriculumView curriculum={e.curriculum} />
    </div>
  )
}
