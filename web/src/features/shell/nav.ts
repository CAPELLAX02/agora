import type { ParseKeys } from 'i18next'
import {
  BookMarkedIcon,
  BookOpenIcon,
  ClipboardCheckIcon,
  GraduationCapIcon,
  ListChecksIcon,
  LockIcon,
  UsersRoundIcon,
  CalendarDaysIcon,
  CalendarRangeIcon,
  ClipboardListIcon,
  FileTextIcon,
  GaugeIcon,
  HomeIcon,
  PresentationIcon,
  LibraryIcon,
  ScrollTextIcon,
  ShieldCheckIcon,
  UserRoundIcon,
  UsersIcon,
  type LucideIcon,
} from 'lucide-react'

import type { Tone } from '@/shared/ui/tone'

type Key = ParseKeys

export type NavItem = {
  to: string
  label: Key
  icon: LucideIcon
  /** tone, öğenin rengidir: menüde ikon kutusu, sayfa başlığında aynı renk. */
  tone: Tone
  /** permission, öğenin görünmesi için gereken yetkidir (dizi: herhangi biri). Boşsa herkes görür. */
  permission?: string | string[]
  /** scoped, yetkinin bir birimi kapsaması gerektiğini söyler (ilişkiye dayalı veriliş yetmez). */
  scoped?: boolean
  end?: boolean
}

export type NavGroup = { label: Key; items: NavItem[] }

export const navGroups: NavGroup[] = [
  {
    label: 'nav.general',
    items: [{ to: '/', label: 'nav.dashboard', tone: 'indigo', icon: HomeIcon, end: true }],
  },
  {
    label: 'nav.academic',
    items: [
      {
        to: '/takvim',
        label: 'nav.calendar',
        tone: 'sky',
        icon: CalendarDaysIcon,
        permission: 'calendar:read',
      },
      {
        to: '/ders-secimi',
        label: 'nav.registration',
        tone: 'violet',
        icon: ClipboardCheckIcon,
        permission: 'registration:manage_own',
      },
      {
        to: '/ders-planim',
        label: 'nav.myCurriculum',
        tone: 'emerald',
        icon: BookMarkedIcon,
        permission: 'registration:manage_own',
      },
      {
        to: '/transkript',
        label: 'nav.transcript',
        tone: 'lime',
        icon: FileTextIcon,
        permission: 'transcript:read_own',
      },
      { to: '/dersler', label: 'nav.catalog', tone: 'amber', icon: BookOpenIcon, permission: 'course:read' },
      {
        to: '/ders-planlari',
        label: 'nav.curricula',
        tone: 'teal',
        icon: LibraryIcon,
        permission: 'curriculum:read',
      },
      {
        to: '/program',
        label: 'nav.schedule',
        tone: 'blue',
        icon: CalendarRangeIcon,
        permission: 'course:read',
      },
      {
        to: '/not-olcegi',
        label: 'nav.gradeScale',
        tone: 'orange',
        icon: GaugeIcon,
        permission: 'curriculum:read',
      },
      {
        to: '/verdigim-dersler',
        label: 'nav.teaching',
        tone: 'fuchsia',
        icon: PresentationIcon,
        permission: 'assessment_plan:manage',
      },
      {
        to: '/ders-acma',
        label: 'nav.offerings',
        tone: 'pink',
        icon: ClipboardListIcon,
        permission: 'offering:manage',
      },
    ],
  },
  {
    label: 'nav.studentAffairs',
    items: [
      {
        to: '/danismanlik',
        label: 'nav.advising',
        tone: 'violet',
        icon: UsersRoundIcon,
        permission: 'registration:approve',
      },
      {
        to: '/ekle-birak-onaylari',
        label: 'nav.addDrop',
        tone: 'teal',
        icon: ListChecksIcon,
        permission: ['registration:approve', 'registration:approve_add_drop'],
      },
      {
        to: '/ogrenciler',
        label: 'nav.students',
        tone: 'sky',
        icon: GraduationCapIcon,
        permission: 'person:read',
        scoped: true,
      },
      { to: '/kayit-engelleri', label: 'nav.holds', tone: 'rose', icon: LockIcon, permission: 'hold:manage' },
    ],
  },
  {
    label: 'nav.account',
    items: [
      { to: '/profil', label: 'nav.profile', tone: 'indigo', icon: UserRoundIcon },
      { to: '/guvenlik', label: 'nav.security', tone: 'emerald', icon: ShieldCheckIcon },
    ],
  },
  {
    label: 'nav.admin',
    items: [
      {
        to: '/yonetim/kullanicilar',
        label: 'nav.users',
        tone: 'blue',
        icon: UsersIcon,
        permission: 'user:read',
      },
      {
        to: '/yonetim/denetim',
        label: 'nav.audit',
        tone: 'slate',
        icon: ScrollTextIcon,
        permission: 'audit:read',
      },
    ],
  },
]

/** visible, menü öğesinin kullanıcının yetkilerine göre görünüp görünmediğini söyler. */
export function visible(
  item: NavItem,
  perms: { hasAny: (p: string | string[]) => boolean; hasScoped: (p: string) => boolean },
): boolean {
  if (!item.permission) {
    return true
  }
  if (item.scoped && typeof item.permission === 'string') {
    return perms.hasScoped(item.permission)
  }
  return perms.hasAny(item.permission)
}

/** navItem, bir yolun menü öğesidir: sayfalar başlıklarında menüdeki ikon ve rengi kullanır. */
export function navItem(to: string): NavItem | undefined {
  for (const g of navGroups) {
    const item = g.items.find((i) => i.to === to)
    if (item) {
      return item
    }
  }
  return undefined
}

/** navTheme, sayfa başlığının ikonu ve rengidir: yan menüdeki öğeyle aynı. */
export function navTheme(to: string): { icon: LucideIcon; tone: Tone } {
  const item = navItem(to)
  return item ? { icon: item.icon, tone: item.tone } : { icon: HomeIcon, tone: 'indigo' }
}
