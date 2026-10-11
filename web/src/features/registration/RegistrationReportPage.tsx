import { ArrowLeftIcon, PrinterIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import { formatDate, formatDateTime } from '@/i18n/format'
import { useGetRegistrationQuery, useListMyProgramsQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Brand } from '@/shared/ui/brand'
import { Button } from '@/shared/ui/button'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

/**
 * RegistrationReportPage, onaylanan ders kaydının yazdırılabilir belgesidir: öğrenci, program,
 * dönem, dersler, AKTS ve kredi toplamları, danışman onayı. Tarayıcının yazdırma penceresiyle
 * PDF'e çevrilebilir. İmzalı ve doğrulama kodlu resmî belge belge altyapısıyla gelecek.
 */
export function RegistrationReportPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [params] = useSearchParams()
  const programs = useListMyProgramsQuery()
  const sp = programs.data?.items.find((p) => p.id === params.get('program')) ?? programs.data?.items[0]
  const { data: r, error, refetch } = useGetRegistrationQuery({ id: sp?.id ?? '' }, { skip: !sp })

  if (error || programs.error) {
    return <ErrorState error={error ?? programs.error} onRetry={() => void refetch()} />
  }
  if (!sp || !r) {
    return <Skeleton className="h-96" />
  }
  const credit = r.items.reduce((sum, it) => sum + it.section.course.credit, 0)

  return (
    <>
      <div className="mb-4 flex flex-wrap justify-between gap-2 print:hidden">
        <Button asChild variant="ghost" size="sm" className="-ml-2">
          <Link to="/ders-secimi">
            <ArrowLeftIcon />
            {t('registration.title')}
          </Link>
        </Button>
        {r.status === 'APPROVED' && (
          <Button onClick={() => window.print()}>
            <PrinterIcon />
            {t('registration.report.print')}
          </Button>
        )}
      </div>
      {r.status !== 'APPROVED' ? (
        <Alert>
          <AlertDescription>{t('registration.report.notApproved')}</AlertDescription>
        </Alert>
      ) : (
        <article
          className="space-y-6 rounded-lg border bg-card p-6 print:border-0 print:p-0"
          aria-label={t('registration.report.title')}
        >
          <header className="flex items-start justify-between gap-4 border-b pb-4">
            <div>
              <h1 className="text-xl font-semibold">{t('registration.report.title')}</h1>
              <p className="text-sm text-muted-foreground">
                {t('registration.report.term', { term: r.term.code })}
              </p>
            </div>
            <Brand />
          </header>
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
            {(
              [
                [t('registration.report.student'), `${r.student.first_name} ${r.student.last_name}`],
                [t('registration.report.studentNo'), r.student.student_no],
                [t('registration.report.faculty'), sp.program.faculty.name_tr],
                [t('registration.report.program'), sp.program.name_tr],
                [
                  t('registration.report.semester'),
                  `${r.student.semester} (${t('registration.report.class', { level: r.student.class_level })})`,
                ],
                [t('registration.report.advisor'), r.student.advisor ?? '—'],
              ] as const
            ).map(([label, value]) => (
              <div key={label} className="flex justify-between gap-4 border-b border-dashed pb-1">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="text-right font-medium">{value}</dd>
              </div>
            ))}
          </dl>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-28">{t('catalog.code')}</TableHead>
                <TableHead>{t('catalog.name')}</TableHead>
                <TableHead className="w-16">{t('registration.report.section')}</TableHead>
                <TableHead className="w-24">{t('registration.report.kind')}</TableHead>
                <TableHead className="w-16 text-right">{t('catalog.credit')}</TableHead>
                <TableHead className="w-16 text-right">{t('catalog.ects')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {r.items.map((it) => (
                <TableRow key={it.id}>
                  <TableCell className="font-mono text-xs">{it.section.course.code}</TableCell>
                  <TableCell>{localized(it.section.course)}</TableCell>
                  <TableCell>{it.section.section_code}</TableCell>
                  <TableCell className="text-xs">{t(`registration.kindsShort.${it.kind}`)}</TableCell>
                  <TableCell className="text-right tabular-nums">{it.section.course.credit}</TableCell>
                  <TableCell className="text-right tabular-nums">{it.ects}</TableCell>
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell colSpan={4}>{t('registration.report.total', { count: r.items.length })}</TableCell>
                <TableCell className="text-right tabular-nums">{credit}</TableCell>
                <TableCell className="text-right tabular-nums">{r.total_ects}</TableCell>
              </TableRow>
            </TableFooter>
          </Table>
          <footer className="space-y-1 border-t pt-4 text-sm">
            <p>
              {t('registration.report.approvedBy', {
                by: r.decided_by ?? '',
                time: formatDateTime(r.decided_at),
              })}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('registration.report.disclaimer', { date: formatDate(new Date()) })}
            </p>
          </footer>
        </article>
      )}
    </>
  )
}
