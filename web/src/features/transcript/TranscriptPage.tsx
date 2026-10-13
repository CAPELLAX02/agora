import { ArrowLeftIcon, PrinterIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'

import { navTheme } from '@/features/shell/nav'
import { useGetTranscriptQuery, useListMyProgramsQuery } from '@/shared/api/generated'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { TranscriptDocument } from './TranscriptDocument'

/**
 * TranscriptPage, transkriptin yazdırılabilir sayfasıdır. Yolda program kaydı varsa onun
 * transkripti (danışman ve öğrenci işleri öğrenci detayından gelir), yoksa öğrencinin kendi
 * programı (birden fazlaysa `?program=`). Tarayıcının yazdırma penceresiyle PDF'e çevrilebilir.
 */
export function TranscriptPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { programId } = useParams()
  const [params] = useSearchParams()
  const mine = useListMyProgramsQuery(undefined, { skip: Boolean(programId) })
  const own = mine.data?.items.find((p) => p.id === params.get('program')) ?? mine.data?.items[0]
  const id = programId ?? own?.id
  const { data, error, refetch } = useGetTranscriptQuery({ id: id ?? '' }, { skip: !id })

  if (error || mine.error) {
    return <ErrorState error={error ?? mine.error} onRetry={() => void refetch()} />
  }
  if (!programId && mine.data && !own) {
    return <EmptyState {...navTheme('/transkript')}>{t('transcript.noProgram')}</EmptyState>
  }
  return (
    <>
      {programId && (
        <Button
          variant="ghost"
          size="sm"
          className="mb-2 -ml-2 print:hidden"
          onClick={() => void navigate(-1)}
        >
          <ArrowLeftIcon />
          {t('common.back')}
        </Button>
      )}
      <PageHeader
        {...navTheme('/transkript')}
        className="print:hidden"
        title={t('transcript.title')}
        description={t('transcript.description')}
        actions={
          <>
            {!programId && mine.data && mine.data.items.length > 1 && (
              <div className="flex flex-wrap gap-1">
                {mine.data.items.map((p) => (
                  <Button key={p.id} asChild size="sm" variant={p.id === own?.id ? 'soft' : 'ghost'}>
                    <Link to={`?program=${p.id}`}>{p.program.code}</Link>
                  </Button>
                ))}
              </div>
            )}
            <Button onClick={() => window.print()} disabled={!data}>
              <PrinterIcon />
              {t('transcript.print')}
            </Button>
          </>
        }
      />
      {data ? <TranscriptDocument transcript={data} /> : <Skeleton className="h-96" />}
    </>
  )
}
