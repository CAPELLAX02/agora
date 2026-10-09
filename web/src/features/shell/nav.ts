import type { ParseKeys } from 'i18next'
import {
  BookOpenIcon,
  CalendarDaysIcon,
  HomeIcon,
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
  /** permission, öğenin görünmesi için gereken yetkidir. Boşsa herkes görür. */
  permission?: string
  end?: boolean
}

export type NavGroup = { label: Key; items: NavItem[] }

export const navGroups: NavGroup[] = [
  { label: 'nav.general', items: [{ to: '/', label: 'nav.dashboard', icon: HomeIcon, end: true }] },
  {
    label: 'nav.academic',
    items: [
      { to: '/takvim', label: 'nav.calendar', icon: CalendarDaysIcon, permission: 'calendar:read' },
      { to: '/dersler', label: 'nav.catalog', icon: BookOpenIcon, permission: 'course:read' },
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
