import { useTranslation } from 'react-i18next'
import { NavLink } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { cn } from '@/shared/lib/utils'
import { Brand } from '@/shared/ui/brand'
import { toneSoft, toneSolid } from '@/shared/ui/tone'

import { navGroups, visible } from './nav'

/** SidebarNav, kullanıcının yetkilerine göre süzülmüş ana menüdür. */
export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation()
  const perms = usePermissions()
  return (
    <nav aria-label={t('nav.main')} className="flex flex-col gap-5 px-3">
      {navGroups.map((group) => {
        const items = group.items.filter((i) => visible(i, perms))
        if (items.length === 0) {
          return null
        }
        return (
          <div key={group.label} className="space-y-1">
            <p className="px-3 pb-1 text-[11px] font-bold tracking-wider text-muted-foreground/80 uppercase">
              {t(group.label)}
            </p>
            <ul className="space-y-0.5">
              {items.map(({ to, label, icon: Icon, tone, end }) => (
                <li key={to}>
                  <NavLink
                    to={to}
                    end={end ?? false}
                    onClick={onNavigate}
                    className={({ isActive }) =>
                      cn(
                        'group flex items-center gap-3 rounded-xl px-2.5 py-1.5 text-sm font-medium transition-all',
                        'outline-none focus-visible:ring-4 focus-visible:ring-ring/30',
                        isActive
                          ? 'bg-sidebar-accent font-semibold text-sidebar-accent-foreground shadow-xs'
                          : 'text-sidebar-foreground/75 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground',
                      )
                    }
                  >
                    {({ isActive }) => (
                      <>
                        <span
                          aria-hidden
                          className={cn(
                            'inline-flex size-7 shrink-0 items-center justify-center rounded-lg transition-all [&>svg]:size-[15px]',
                            isActive
                              ? cn(toneSolid[tone], 'shadow-sm')
                              : cn(toneSoft[tone], 'group-hover:scale-105'),
                          )}
                        >
                          <Icon />
                        </span>
                        {t(label)}
                      </>
                    )}
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
      <div className="px-5">
        <Brand />
      </div>
      <SidebarNav />
    </aside>
  )
}
