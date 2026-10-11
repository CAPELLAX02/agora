import { ArrowRightIcon, ClipboardCheckIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { EctsMeter } from '@/features/registration/Basket'
import { RegistrationStatusBadge } from '@/features/registration/badges'
import { useGetRegistrationQuery, useListMyProgramsQuery } from '@/shared/api/generated'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Skeleton } from '@/shared/ui/skeleton'

/**
 * RegistrationCard, öğrencinin içinde bulunulan dönemdeki ders seçme durumudur: sepet ve AKTS,
 * danışman kararı, ders seçme penceresi açıksa ders seçimine kısayol.
 */
export function RegistrationCard() {
  const { t } = useTranslation()
  const programs = useListMyProgramsQuery()
  const program = programs.data?.items[0]
  const { data: r, isError } = useGetRegistrationQuery({ id: program?.id ?? '' }, { skip: !program })
  if (programs.data && !program) {
    return null
  }
  if (isError) {
    return null // aktif dönem yoksa kart gösterilmez
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ClipboardCheckIcon className="size-4 text-primary" aria-hidden />
          {t('dashboard.registration.title')}
        </CardTitle>
        {r && <CardDescription>{t('registration.termTitle', { term: r.term.code })}</CardDescription>}
      </CardHeader>
      <CardContent className="space-y-4">
        {!r ? (
          <Skeleton className="h-20" />
        ) : (
          <>
            <div className="flex items-center justify-between gap-2 text-sm">
              <RegistrationStatusBadge status={r.status} />
              <span className="text-muted-foreground">
                {t('dashboard.registration.courses', { count: r.items.length })}
              </span>
            </div>
            <EctsMeter total={r.total_ects} limit={r.ects_limit} />
            {r.status === 'REJECTED' && r.decision_note && (
              <p className="text-sm text-destructive">{r.decision_note}</p>
            )}
            <Button variant="outline" size="sm" asChild>
              <Link to="/ders-secimi">
                {r.actions.can_edit ? t('dashboard.registration.go') : t('dashboard.registration.view')}
                <ArrowRightIcon />
              </Link>
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  )
}
