import { ChevronDownIcon, ChevronRightIcon, LinkIcon, Trash2Icon } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { CourseKindBadge } from '@/features/catalog/course'
import { ProgressBadge, ProgressSummary } from '@/features/registration/ProgressView'
import {
  useGetElectiveGroupQuery,
  type CurriculumDetail,
  type CurriculumItem,
  type ProgressLine,
  type StudentProgress,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardHeader, CardTitle } from '@/shared/ui/card'
import { LetterGrade } from '@/shared/ui/letter-grade'
import { Skeleton } from '@/shared/ui/skeleton'
import { StatTile } from '@/shared/ui/stat-tile'
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { toneSolid, type Tone } from '@/shared/ui/tone'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/shared/ui/tooltip'

const semesterTones: Tone[] = ['indigo', 'violet', 'sky', 'teal', 'emerald', 'amber', 'orange', 'pink']

/** Ders durumunun rengi (satırın başındaki nokta). */
const statusDot: Record<ProgressLine['status'], string> = {
  PASSED: 'bg-emerald-500',
  FAILED: 'bg-rose-500',
  IN_PROGRESS: 'bg-sky-500',
  PARTIAL: 'bg-amber-500',
  NOT_TAKEN: 'bg-slate-300 dark:bg-slate-600',
}

/**
 * CurriculumView, bir müfredat sürümünü yarıyıl yarıyıl gösterir: zorunlu dersler, seçmeli
 * yuvalar (havuzun dersleri açılabilir) ve mezuniyet özeti. onRemove verilirse (taslak
 * düzenlenirken) satırlar kaldırılabilir. progress verilirse (öğrencinin kendi planı) her
 * satırda dersin durumu (geçti, kaldı, devam ediyor) ve mezuniyet ilerlemesi gösterilir.
 */
export function CurriculumView({
  curriculum,
  onRemove,
  progress,
}: {
  curriculum: CurriculumDetail
  onRemove?: (item: CurriculumItem) => void
  progress?: StudentProgress | undefined
}) {
  const { t } = useTranslation()
  const lines = new Map(progress?.lines.map((l) => [l.item_id, l]))
  const semesters = new Map<number, CurriculumItem[]>()
  for (const it of curriculum.items) {
    semesters.set(it.semester_no, [...(semesters.get(it.semester_no) ?? []), it])
  }
  const totals = new Map(curriculum.summary.semesters.map((s) => [s.semester_no, s]))

  return (
    <div className="space-y-6">
      {progress && <ProgressSummary progress={progress} />}
      <Summary curriculum={curriculum} />
      {semesters.size === 0 ? (
        <p className="text-sm text-muted-foreground">{t('curriculum.view.empty')}</p>
      ) : (
        <div className="grid gap-6 xl:grid-cols-2">
          {[...semesters.entries()]
            .sort(([a], [b]) => a - b)
            .map(([no, items]) => (
              <Card
                key={no}
                role="region"
                className="gap-0 overflow-hidden py-0"
                aria-label={t('curriculum.view.semester', { no })}
              >
                <CardHeader className="border-b border-border/60 py-3">
                  <CardTitle className="flex items-center justify-between gap-2 text-base">
                    <span className="flex items-center gap-2.5">
                      <span
                        aria-hidden
                        className={cn(
                          'inline-flex size-8 items-center justify-center rounded-xl text-sm font-extrabold',
                          toneSolid[semesterTones[(no - 1) % semesterTones.length]!],
                        )}
                      >
                        {no}
                      </span>
                      {t('curriculum.view.semester', { no })}
                    </span>
                    <span className="rounded-full bg-muted px-2.5 py-0.5 text-xs font-semibold text-muted-foreground">
                      {t('curriculum.view.yearSeason', {
                        year: Math.ceil(no / 2),
                        season: t(no % 2 === 1 ? 'curriculum.view.fall' : 'curriculum.view.spring'),
                      })}
                    </span>
                  </CardTitle>
                </CardHeader>
                <Table className="table-fixed">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-28">{t('catalog.code')}</TableHead>
                      <TableHead>{t('catalog.name')}</TableHead>
                      <TableHead className="w-14 text-right">{t('catalog.hours')}</TableHead>
                      <TableHead className="w-16 text-right">{t('catalog.credit')}</TableHead>
                      <TableHead className="w-14 text-right">{t('catalog.ects')}</TableHead>
                      {onRemove && <TableHead className="w-10" />}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {items.map((it) => (
                      <ItemRow key={it.id} item={it} onRemove={onRemove} line={lines.get(it.id)} />
                    ))}
                  </TableBody>
                  <TableFooter>
                    <TableRow>
                      <TableCell colSpan={3} className="text-xs">
                        {t('curriculum.view.semesterTotal')}
                      </TableCell>
                      <TableCell className="text-right">{totals.get(no)?.national_credit}</TableCell>
                      <TableCell className="text-right">{totals.get(no)?.ects}</TableCell>
                      {onRemove && <TableCell />}
                    </TableRow>
                  </TableFooter>
                </Table>
              </Card>
            ))}
        </div>
      )}
    </div>
  )
}

function Summary({ curriculum }: { curriculum: CurriculumDetail }) {
  const { t } = useTranslation()
  const s = curriculum.summary
  const tiles: [string, string | number][] = [
    [t('curriculum.view.total'), `${s.total_ects} / ${curriculum.total_ects_required}`],
    [t('curriculum.view.compulsory'), s.compulsory_ects],
    [t('curriculum.view.elective'), s.elective_ects],
    ...s.elective_kinds.map((k): [string, number] => [t(`catalog.groupKinds.${k.kind}`), k.ects]),
  ]
  return (
    <dl
      className="grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-3"
      aria-label={t('curriculum.view.summary')}
    >
      {tiles.map(([label, value], i) => (
        <StatTile
          key={label}
          label={label}
          value={value}
          suffix={t('catalog.ects')}
          tone={semesterTones[i % semesterTones.length]}
        />
      ))}
    </dl>
  )
}

function ItemRow({
  item,
  onRemove,
  line,
}: {
  item: CurriculumItem
  onRemove?: ((item: CurriculumItem) => void) | undefined
  line?: ProgressLine | undefined
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [open, setOpen] = useState(false)
  const slot = item.item_type === 'ELECTIVE_SLOT'
  const span = onRemove ? 6 : 5

  return (
    <Fragment>
      <TableRow className={slot ? 'bg-violet-500/[0.04]' : undefined}>
        <TableCell className="font-mono text-xs font-semibold">
          <span className="flex items-center gap-2">
            {line && (
              <span aria-hidden className={cn('size-2 shrink-0 rounded-full', statusDot[line.status])} />
            )}
            <span className="truncate">{item.code}</span>
          </span>
        </TableCell>
        <TableCell className="whitespace-normal">
          <div className="flex flex-wrap items-center gap-2">
            {slot ? (
              <Button
                variant="ghost"
                size="sm"
                className="-ml-2 h-auto px-2 py-0.5 text-left font-medium whitespace-normal"
                aria-expanded={open}
                onClick={() => setOpen(!open)}
              >
                {open ? <ChevronDownIcon /> : <ChevronRightIcon />}
                {localized(item)}
              </Button>
            ) : item.course ? (
              <Link to={`/dersler/${item.course.id}`} className="font-medium hover:underline">
                {localized(item)}
              </Link>
            ) : (
              localized(item)
            )}
            {slot && (
              <Badge variant="secondary">
                {t('curriculum.view.electiveCount', { count: item.course_count })}
              </Badge>
            )}
            {item.course_kind && <CourseKindBadge kind={item.course_kind} />}
            {line && <ProgressBadge status={line.status} />}
            {line?.last_attempt?.letter && (
              <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                <LetterGrade letter={line.last_attempt.letter} />
                {line.last_attempt.course_code !== item.code && `(${line.last_attempt.course_code})`}
              </span>
            )}
            {line && slot && line.filled.length + line.in_progress.length > 0 && (
              <span className="text-xs text-muted-foreground">
                {[...line.filled, ...line.in_progress].map((a) => a.course_code).join(', ')}
              </span>
            )}
            {item.has_prerequisites && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <LinkIcon
                    className="size-3.5 text-muted-foreground"
                    aria-label={t('curriculum.view.hasPrerequisites')}
                  />
                </TooltipTrigger>
                <TooltipContent>{t('curriculum.view.hasPrerequisites')}</TooltipContent>
              </Tooltip>
            )}
          </div>
        </TableCell>
        <TableCell className="text-right font-mono text-xs">
          {item.theory_hours}-{item.practice_hours}
        </TableCell>
        <TableCell className="text-right">{item.national_credit}</TableCell>
        <TableCell className="text-right">{item.ects}</TableCell>
        {onRemove && (
          <TableCell className="text-right">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onRemove(item)}
              aria-label={t('curriculum.edit.removeItem', { code: item.code })}
            >
              <Trash2Icon />
            </Button>
          </TableCell>
        )}
      </TableRow>
      {slot && open && item.elective_group && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={span} className="bg-muted/20">
            <Pool groupId={item.elective_group.id} />
          </TableCell>
        </TableRow>
      )}
    </Fragment>
  )
}

function Pool({ groupId }: { groupId: string }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { data } = useGetElectiveGroupQuery({ id: groupId })
  if (!data) {
    return <Skeleton className="h-16" />
  }
  if (data.courses.length === 0) {
    return <p className="text-xs text-muted-foreground">{t('curriculum.view.emptyPool')}</p>
  }
  return (
    <ul
      className="grid gap-1 sm:grid-cols-2"
      aria-label={t('curriculum.view.pool', { name: localized(data) })}
    >
      {data.courses.map((c) => (
        <li key={c.id} className="text-xs">
          <Link to={`/dersler/${c.id}`} className="hover:underline">
            <span className="font-mono text-muted-foreground">{c.code}</span> {localized(c)}
          </Link>{' '}
          <span className="text-muted-foreground">· {t('curriculum.view.ectsValue', { ects: c.ects })}</span>
        </li>
      ))}
    </ul>
  )
}
