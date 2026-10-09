import { useTranslation } from 'react-i18next'
import { NavLink } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { cn } from '@/shared/lib/utils'
import { Brand } from '@/shared/ui/brand'

import { navGroups } from './nav'

/** SidebarNav, kullanıcının yetkilerine göre süzülmüş ana menüdür. */
export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { has } = usePermissions()
  return (
    <nav aria-label={t('nav.main')} className="flex flex-col gap-6 px-3">
      {navGroups.map((group) => {
        const items = group.items.filter((i) => !i.permission || has(i.permission))
        if (items.length === 0) {
          return null
        }
        return (
          <div key={group.label} className="space-y-1">
            <p className="px-3 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              {t(group.label)}
            </p>
            <ul className="space-y-0.5">
              {items.map(({ to, label, icon: Icon, end }) => (
                <li key={to}>
                  <NavLink
                    to={to}
                    end={end ?? false}
                    onClick={onNavigate}
                    className={({ isActive }) =>
                      cn(
                        'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                        'outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
                        isActive
                          ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                          : 'text-sidebar-foreground/80 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground',
                      )
                    }
                  >
                    <Icon className="size-4" aria-hidden />
                    {t(label)}
                  </NavLink>
                </li>
              ))}
            </ul>
          </div>
        )
      })}
    </nav>
  )
}

export function Sidebar() {
  return (
    <aside className="sticky top-0 hidden h-svh w-64 shrink-0 flex-col gap-6 overflow-y-auto border-r border-sidebar-border bg-sidebar py-5 lg:flex">
      <div className="px-6">
        <Brand />
      </div>
      <SidebarNav />
    </aside>
  )
}
