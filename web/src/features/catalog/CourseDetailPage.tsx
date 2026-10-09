import { ArrowLeftIcon, ArrowRightLeftIcon } from 'lucide-react'
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'

import { useGetCourseQuery, type CourseDetail, type CourseRef } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { DescriptionList } from '@/shared/ui/description-list'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { CourseKindBadge } from './course'
import { hours } from './format'

/**
 * CourseDetailPage, bir dersin katalog bilgisini, ön koşullarını, onu isteyen dersleri,
 * eski ve yeni kodlarla eşdeğerliklerini ve bulunduğu seçmeli havuzları gösterir.
 */
export function CourseDetailPage() {
  const { t, i18n } = useTranslation()
  const { id = '' } = useParams()
  const localized = useLocalized()
  const { data: course, error, refetch } = useGetCourseQuery({ id })

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!course) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-80" />
        <Skeleton className="h-48" />
      </div>
    )
  }

  const en = i18n.language === 'en'
  const description = (en ? course.description_en : course.description_tr) ?? course.description_tr

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
        <Link to="/dersler">
          <ArrowLeftIcon />
          {t('catalog.title')}
        </Link>
      </Button>
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            <span className="font-mono text-muted-foreground">{course.code}</span>
            {localized(course)}
          </span>
        }
        description={en ? course.name_tr : course.name_en}
        actions={
          <div className="flex gap-2">
            <CourseKindBadge kind={course.kind} />
            {!course.is_active && <Badge variant="muted">{t('catalog.inactive')}</Badge>}
          </div>
        }
      />
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>{t('catalog.detail.info')}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-6">
              <DescriptionList
                items={[
                  [t('catalog.detail.hours'), `${hours(course)} (${t('catalog.detail.hoursHint')})`],
                  [t('catalog.credit'), course.national_credit],
                  [t('catalog.ects'), course.ects],
                  [t('catalog.detail.language'), t(`catalog.languages.${course.language}`)],
                  [t('catalog.detail.grading'), t(`catalog.grading.${course.grading_mode}`)],
                  [
                    t('catalog.owner'),
                    course.owner_department
                      ? localized(course.owner_department)
                      : t('catalog.universityCommon'),
                  ],
                ]}
              />
              {description && <p className="text-sm leading-relaxed">{description}</p>}
              {course.learning_outcomes.length > 0 && (
                <div className="space-y-2">
                  <h3 className="text-sm font-medium">{t('catalog.detail.outcomes')}</h3>
                  <ol className="list-decimal space-y-1 pl-5 text-sm">
                    {course.learning_outcomes.map((o, i) => (
                      <li key={i}>{o}</li>
                    ))}
                  </ol>
                </div>
              )}
            </CardContent>
          </Card>
          <Prerequisites course={course} />
          <Equivalences course={course} />
        </div>
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>{t('catalog.detail.groups')}</CardTitle>
              <CardDescription>{t('catalog.detail.groupsHint')}</CardDescription>
            </CardHeader>
            <CardContent>
              {course.elective_groups.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t('catalog.detail.noGroups')}</p>
              ) : (
                <ul className="space-y-3">
                  {course.elective_groups.map((g) => (
                    <li key={g.id}>
                      <p className="text-sm font-medium">{localized(g)}</p>
                      <p className="text-xs text-muted-foreground">
                        <span className="font-mono">{g.code}</span> · {t(`catalog.groupKinds.${g.kind}`)}
                      </p>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('catalog.detail.requiredBy')}</CardTitle>
            </CardHeader>
            <CardContent>
              {course.required_by.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t('catalog.detail.noRequiredBy')}</p>
              ) : (
                <ul className="space-y-2">
                  {course.required_by.map((c) => (
                    <li key={c.id}>
                      <CourseLink course={c} />
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  )
}

function CourseLink({ course }: { course: CourseRef }) {
  const localized = useLocalized()
  return (
    <Link to={`/dersler/${course.id}`} className="text-sm hover:underline">
      <span className="font-mono text-xs text-muted-foreground">{course.code}</span> {localized(course)}
    </Link>
  )
}

/**
 * Prerequisites, ön koşulları gösterir: aynı gruptaki dersler VEYA, farklı gruplar VE ile
 * bağlanır ("COM1002 ve (MTH0143 veya MTH0141)").
 */
function Prerequisites({ course }: { course: CourseDetail }) {
  const { t } = useTranslation()
  const groups = new Map<number, CourseDetail['prerequisites']>()
  for (const p of course.prerequisites) {
    groups.set(p.group_no, [...(groups.get(p.group_no) ?? []), p])
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('catalog.detail.prerequisites')}</CardTitle>
        {groups.size > 1 && <CardDescription>{t('catalog.detail.prerequisitesHint')}</CardDescription>}
      </CardHeader>
      <CardContent>
        {groups.size === 0 ? (
          <p className="text-sm text-muted-foreground">{t('catalog.detail.noPrerequisites')}</p>
        ) : (
          <ol className="space-y-3">
            {[...groups.entries()].map(([no, items], gi) => (
              <Fragment key={no}>
                {gi > 0 && (
                  <li
                    aria-hidden
                    className="text-xs font-semibold tracking-wide text-muted-foreground uppercase"
                  >
                    {t('catalog.detail.and')}
                  </li>
                )}
                <li className="rounded-md border p-3">
                  <ul className="space-y-2">
                    {items.map((p, i) => (
                      <li key={p.course.id} className="flex flex-wrap items-center gap-2">
                        {i > 0 && (
                          <span className="text-xs text-muted-foreground">{t('catalog.detail.or')}</span>
                        )}
                        <CourseLink course={p.course} />
                        <Badge variant="muted">{t(`catalog.requirements.${p.requirement}`)}</Badge>
                      </li>
                    ))}
                  </ul>
                </li>
              </Fragment>
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  )
}

function Equivalences({ course }: { course: CourseDetail }) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowRightLeftIcon className="size-4" aria-hidden />
          {t('catalog.detail.equivalences')}
        </CardTitle>
        <CardDescription>{t('catalog.detail.equivalencesHint')}</CardDescription>
      </CardHeader>
      <CardContent>
        {course.equivalences.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('catalog.detail.noEquivalences')}</p>
        ) : (
          <ul className="space-y-3">
            {course.equivalences.map((e) => (
              <li key={e.id} className="space-y-1">
                <p className="text-xs text-muted-foreground">{t(`catalog.relations.${e.relation}`)}</p>
                <CourseLink course={e.course} />
                {(e.valid_from_year || e.note) && (
                  <p className="text-xs text-muted-foreground">
                    {[e.valid_from_year && t('catalog.detail.validFrom', { year: e.valid_from_year }), e.note]
                      .filter(Boolean)
                      .join(' · ')}
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
