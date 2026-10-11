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

type Key = ParseKeys

export type NavItem = {
  to: string
  label: Key
  icon: LucideIcon
  /** permission, öğenin görünmesi için gereken yetkidir (dizi: herhangi biri). Boşsa herkes görür. */
  permission?: string | string[]
  /** scoped, yetkinin bir birimi kapsaması gerektiğini söyler (ilişkiye dayalı veriliş yetmez). */
  scoped?: boolean
  end?: boolean
}

export type NavGroup = { label: Key; items: NavItem[] }

export const navGroups: NavGroup[] = [
  { label: 'nav.general', items: [{ to: '/', label: 'nav.dashboard', icon: HomeIcon, end: true }] },
  {
    label: 'nav.academic',
    items: [
      { to: '/takvim', label: 'nav.calendar', icon: CalendarDaysIcon, permission: 'calendar:read' },
      {
        to: '/ders-secimi',
        label: 'nav.registration',
        icon: ClipboardCheckIcon,
        permission: 'registration:manage_own',
      },
      {
        to: '/ders-planim',
        label: 'nav.myCurriculum',
        icon: BookMarkedIcon,
        permission: 'registration:manage_own',
      },
      { to: '/dersler', label: 'nav.catalog', icon: BookOpenIcon, permission: 'course:read' },
      { to: '/ders-planlari', label: 'nav.curricula', icon: LibraryIcon, permission: 'curriculum:read' },
      { to: '/program', label: 'nav.schedule', icon: CalendarRangeIcon, permission: 'course:read' },
      { to: '/not-olcegi', label: 'nav.gradeScale', icon: GaugeIcon, permission: 'curriculum:read' },
      {
        to: '/verdigim-dersler',
        label: 'nav.teaching',
        icon: PresentationIcon,
        permission: 'assessment_plan:manage',
      },
      { to: '/ders-acma', label: 'nav.offerings', icon: ClipboardListIcon, permission: 'offering:manage' },
    ],
  },
  {
    label: 'nav.studentAffairs',
    items: [
      { to: '/danismanlik', label: 'nav.advising', icon: UsersRoundIcon, permission: 'registration:approve' },
      {
        to: '/ekle-birak-onaylari',
        label: 'nav.addDrop',
        icon: ListChecksIcon,
        permission: ['registration:approve', 'registration:approve_add_drop'],
      },
      {
        to: '/ogrenciler',
        label: 'nav.students',
        icon: GraduationCapIcon,
        permission: 'person:read',
        scoped: true,
      },
      { to: '/kayit-engelleri', label: 'nav.holds', icon: LockIcon, permission: 'hold:manage' },
    ],
  },
  {
    label: 'nav.account',
    items: [
      { to: '/profil', label: 'nav.profile', icon: UserRoundIcon },
      { to: '/guvenlik', label: 'nav.security', icon: ShieldCheckIcon },
    ],
  },
  {
    label: 'nav.admin',
    items: [
      { to: '/yonetim/kullanicilar', label: 'nav.users', icon: UsersIcon, permission: 'user:read' },
      { to: '/yonetim/denetim', label: 'nav.audit', icon: ScrollTextIcon, permission: 'audit:read' },
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
