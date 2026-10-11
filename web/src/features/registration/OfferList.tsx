import {
  ArrowLeftRightIcon,
  CheckIcon,
  ChevronDownIcon,
  LinkIcon,
  PlusIcon,
  SearchIcon,
  UserRoundIcon,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import type { RegistrationOffer, RegistrationSection } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card } from '@/shared/ui/card'
import { IconChip } from '@/shared/ui/icon-chip'
import { Input } from '@/shared/ui/input'
import { EmptyState } from '@/shared/ui/states'
import { toneEdge, toneFor, toneSoft, type Tone } from '@/shared/ui/tone'

import { ItemKindBadge } from './badges'
import { categoryStyle } from './categories'
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
  /** kind, düğmenin görünümüdür: ekle, değiştir ya da (devre dışı) sepette. */
  kind?: 'add' | 'switch' | 'done'
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
        <SearchIcon className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          className="h-11 rounded-xl pl-10"
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
        const { tone, icon } = categoryStyle[cat]
        return (
          <Card
            key={cat}
            role="region"
            aria-label={t(`registration.categories.${cat}`)}
            className="gap-0 overflow-hidden py-0"
          >
            <button
              type="button"
              className="flex w-full cursor-pointer items-center gap-3 px-4 py-3.5 text-left transition-colors outline-none hover:bg-muted/40 focus-visible:bg-muted/40"
              aria-expanded={expanded}
              onClick={() => setOpen({ ...open, [cat]: !open[cat] })}
            >
              <IconChip icon={icon} tone={tone} solid />
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2 text-base font-bold">
                  {t(`registration.categories.${cat}`)}
                  <span className={cn('rounded-full px-2 py-0.5 text-xs font-bold', toneSoft[tone])}>
                    {items.length}
                  </span>
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`registration.categoryHints.${cat}`)}
                </span>
              </span>
              <ChevronDownIcon
                className={cn(
                  'size-5 shrink-0 text-muted-foreground transition-transform',
                  !expanded && '-rotate-90',
                )}
                aria-hidden
              />
            </button>
            {expanded && (
              <div className="space-y-3 border-t border-border/60 bg-muted/20 p-3 sm:p-4">
                {items.map((o) => (
                  <CourseOffer key={o.course.id} offer={o} tone={tone} action={action} />
                ))}
              </div>
            )}
          </Card>
        )
      })}
    </div>
  )
}

function CourseOffer({
  offer: o,
  tone,
  action,
}: {
  offer: Offer
  tone: Tone
  action: (offer: Offer, section: RegistrationSection) => OfferAction | null
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const text = useViolationText()
  const replaces = o.line.item_type === 'COURSE' && o.line.code !== o.course.code
  const inBasket = Boolean(o.basket_item_id)
  return (
    <section
      className={cn(
        'relative space-y-3 overflow-hidden rounded-xl border bg-card p-4 pl-5 shadow-xs transition-shadow hover:shadow-soft',
        'before:absolute before:inset-y-0 before:left-0 before:w-1.5',
        toneEdge[tone],
        inBasket ? 'border-emerald-400/50 ring-1 ring-emerald-400/30' : 'border-border/70',
      )}
      aria-label={`${o.course.code} ${localized(o.course)}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span
          className={cn(
            'rounded-md px-1.5 py-0.5 font-mono text-xs font-bold',
            toneSoft[toneFor(o.course.code)],
          )}
        >
          {o.course.code}
        </span>
        <Link to={`/dersler/${o.course.id}`} className="font-bold hover:text-primary hover:underline">
          {localized(o.course)}
        </Link>
        <Badge variant="outline">{t('registration.ects', { ects: o.course.ects })}</Badge>
        <ItemKindBadge kind={o.kind} />
        {inBasket && (
          <Badge variant="success">
            <CheckIcon />
            {t('registration.offer.inBasket')}
          </Badge>
        )}
      </div>
      <p className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs text-muted-foreground">
        <span>
          {o.line.item_type === 'ELECTIVE_SLOT'
            ? t('registration.offer.slot', {
                code: o.line.code,
                semester: o.line.semester_no,
                count: o.line.remaining,
              })
            : t('registration.offer.line', { semester: o.line.semester_no })}
        </span>
        {replaces && (
          <span className="inline-flex items-center gap-1 rounded-full bg-sky-500/10 px-2 py-0.5 font-semibold text-sky-700 dark:text-sky-300">
            <LinkIcon className="size-3" aria-hidden />
            {t('registration.offer.replaces', { code: o.line.code })}
          </span>
        )}
        {o.last_letter && (
          <span className="rounded-full bg-muted px-2 py-0.5 font-semibold">
            {t('registration.offer.lastLetter', { letter: o.last_letter })}
          </span>
        )}
      </p>
      {o.prerequisite && (
        <p className="rounded-lg bg-rose-500/8 px-3 py-2 text-xs font-medium text-rose-700 dark:text-rose-300">
          {text(o.prerequisite)}
        </p>
      )}
      <ul className="space-y-2">
        {o.sections.map((s) => (
          <SectionRow key={s.id} section={s} action={action(o, s)} />
        ))}
      </ul>
    </section>
  )
}

function SectionRow({ section: s, action: a }: { section: RegistrationSection; action: OfferAction | null }) {
  const ratio = s.capacity > 0 ? s.enrolled / s.capacity : 1
  const seatTone = !s.available ? 'bg-rose-500' : ratio >= 0.85 ? 'bg-amber-500' : 'bg-emerald-500'
  const kind = a?.kind ?? 'add'
  const Icon = kind === 'done' ? CheckIcon : kind === 'switch' ? ArrowLeftRightIcon : PlusIcon
  return (
    <li className="flex flex-col gap-3 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5 text-xs sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1 space-y-1.5">
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-sm font-bold">{s.label}</span>
          {s.instructors.length > 0 && (
            <span className="inline-flex items-center gap-1 text-muted-foreground">
              <UserRoundIcon className="size-3.5" aria-hidden />
              {s.instructors.join(', ')}
            </span>
          )}
        </p>
        <p className="text-foreground/80">
          <SectionSchedule section={s} />
        </p>
        <div className="flex items-center gap-2">
          <span aria-hidden className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
            <span
              className={cn('block h-full rounded-full', seatTone)}
              style={{ width: `${Math.min(ratio, 1) * 100}%` }}
            />
          </span>
          <SectionSeats section={s} />
        </div>
      </div>
      {a && a.label && (
        <Button
          size="sm"
          variant={kind === 'done' ? 'secondary' : kind === 'switch' ? 'outline' : 'soft'}
          className="shrink-0 self-start sm:self-center"
          disabled={a.disabled}
          onClick={a.onClick}
          aria-label={`${s.label}: ${a.label}`}
        >
          <Icon aria-hidden />
          {a.label}
        </Button>
      )}
    </li>
  )
}
