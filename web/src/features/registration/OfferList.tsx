import { ChevronDownIcon, ChevronRightIcon, LinkIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import type { RegistrationOffer, RegistrationSection } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { Input } from '@/shared/ui/input'
import { EmptyState } from '@/shared/ui/states'

import { ItemKindBadge } from './badges'
import { SectionSchedule, SectionSeats } from './SectionInfo'
import { useViolationText } from './shared'

type Offer = RegistrationOffer['items'][number]
type Category = Offer['category']

const order: Category[] = ['REPEAT', 'CURRENT', 'ELECTIVE', 'UPPER', 'UPGRADE']

/** OfferAction, bir şube için gösterilecek düğmedir. */
export type OfferAction = {
  /** label, düğmenin metni; boşsa düğme gösterilmez. */
  label: string
  disabled?: boolean
  onClick: () => void
}

/**
 * OfferList, öğrencinin bu dönem alabileceği dersleri kategorilere göre gösterir: alttan
 * dersler, dönem dersleri, seçmeliler, üst yarıyıl dersleri ve (kapalı başlayan) not
 * yükseltme. Her şubenin programı, doluluğu ve öğretim elemanı görünür; action şube için
 * düğmeyi verir (sepete ekle, şube değiştir, ekle-bırak talebine ekle...).
 */
export function OfferList({
  offer,
  action,
}: {
  offer: RegistrationOffer
  action: (offer: Offer, section: RegistrationSection) => OfferAction | null
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState<Record<Category, boolean>>({
    REPEAT: true,
    CURRENT: true,
    ELECTIVE: true,
    UPPER: false,
    UPGRADE: false,
  })

  const q = query.trim().toLocaleLowerCase('tr')
  const visible = offer.items.filter(
    (o) => !q || `${o.course.code} ${localized(o.course)} ${o.line.code}`.toLocaleLowerCase('tr').includes(q),
  )

  return (
    <div className="space-y-4">
      <div className="relative">
        <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          className="pl-9"
          placeholder={t('registration.offer.search')}
          aria-label={t('registration.offer.search')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>
      {offer.items.length === 0 && <EmptyState>{t('registration.offer.empty')}</EmptyState>}
      {order.map((cat) => {
        const items = visible.filter((o) => o.category === cat)
        if (items.length === 0) {
          return null
        }
        const expanded = open[cat] || q !== ''
        return (
          <Card
            key={cat}
            role="region"
            aria-label={t(`registration.categories.${cat}`)}
            className="gap-0 py-0"
          >
            <CardHeader className="py-3">
              <CardTitle className="text-base">
                <Button
                  variant="ghost"
                  size="sm"
                  className="-ml-2 h-auto px-2 py-1 text-base font-semibold"
                  aria-expanded={expanded}
                  onClick={() => setOpen({ ...open, [cat]: !open[cat] })}
                >
                  {expanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
                  {t(`registration.categories.${cat}`)}
                  <Badge variant="muted">{items.length}</Badge>
                </Button>
              </CardTitle>
              <p className="text-xs text-muted-foreground">{t(`registration.categoryHints.${cat}`)}</p>
            </CardHeader>
            {expanded && (
              <CardContent className="divide-y border-t px-0">
                {items.map((o) => (
                  <CourseOffer key={o.course.id} offer={o} action={action} />
                ))}
              </CardContent>
            )}
          </Card>
        )
      })}
    </div>
  )
}

function CourseOffer({
  offer: o,
  action,
}: {
  offer: Offer
  action: (offer: Offer, section: RegistrationSection) => OfferAction | null
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const text = useViolationText()
  const replaces = o.line.item_type === 'COURSE' && o.line.code !== o.course.code
  return (
    <section className="space-y-2 px-4 py-3" aria-label={`${o.course.code} ${localized(o.course)}`}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-xs text-muted-foreground">{o.course.code}</span>
        <Link to={`/dersler/${o.course.id}`} className="font-medium hover:underline">
          {localized(o.course)}
        </Link>
        <Badge variant="outline">{t('registration.ects', { ects: o.course.ects })}</Badge>
        <ItemKindBadge kind={o.kind} />
        {o.basket_item_id && <Badge variant="success">{t('registration.offer.inBasket')}</Badge>}
      </div>
      <p className="text-xs text-muted-foreground">
        {o.line.item_type === 'ELECTIVE_SLOT'
          ? t('registration.offer.slot', {
              code: o.line.code,
              semester: o.line.semester_no,
              count: o.line.remaining,
            })
          : t('registration.offer.line', { semester: o.line.semester_no })}
        {replaces && (
          <>
            {' · '}
            <LinkIcon className="inline size-3" aria-hidden />{' '}
            {t('registration.offer.replaces', { code: o.line.code })}
          </>
        )}
        {o.last_letter && ` · ${t('registration.offer.lastLetter', { letter: o.last_letter })}`}
      </p>
      {o.prerequisite && <p className="text-xs text-destructive">{text(o.prerequisite)}</p>}
      <ul className="space-y-1.5">
        {o.sections.map((s) => {
          const a = action(o, s)
          return (
            <li
              key={s.id}
              className="flex flex-col gap-2 rounded-md border bg-muted/20 px-3 py-2 text-xs sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="min-w-0 space-y-0.5">
                <p className="font-medium">
                  {s.label}
                  {s.instructors.length > 0 && (
                    <span className="font-normal text-muted-foreground"> · {s.instructors.join(', ')}</span>
                  )}
                </p>
                <p>
                  <SectionSchedule section={s} />
                </p>
                <p>
                  <SectionSeats section={s} />
                </p>
              </div>
              {a && a.label && (
                <Button
                  size="sm"
                  variant="outline"
                  className="shrink-0"
                  disabled={a.disabled}
                  onClick={a.onClick}
                  aria-label={`${s.label}: ${a.label}`}
                >
                  {a.label}
                </Button>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
