import {
  BellRingIcon,
  LanguagesIcon,
  LogOutIcon,
  PaletteIcon,
  ShieldCheckIcon,
  UserRoundIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'

import { useLogout } from '@/features/auth/useLogout'
import { useGetMeQuery } from '@/shared/api/generated'
import { Avatar } from '@/shared/ui/avatar'
import { Button } from '@/shared/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { Skeleton } from '@/shared/ui/skeleton'

import { LanguageItems, ThemeItems } from './Preferences'

export function UserMenu() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const logout = useLogout()
  const { data: me } = useGetMeQuery()

  if (!me) {
    return <Skeleton className="size-9 rounded-full" />
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="h-11 gap-2.5 rounded-xl px-2" aria-label={t('nav.userMenu')}>
          <Avatar firstName={me.first_name} lastName={me.last_name} className="size-9" />
          <span className="hidden text-left text-sm leading-tight sm:block">
            <span className="block font-bold">
              {me.first_name} {me.last_name}
            </span>
            <span className="block text-xs text-muted-foreground">{me.username}</span>
          </span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="flex items-center gap-3 font-normal">
          <Avatar firstName={me.first_name} lastName={me.last_name} className="size-10" />
          <span className="min-w-0">
            <span className="block font-bold">
              {me.first_name} {me.last_name}
            </span>
            <span className="block truncate text-xs text-muted-foreground">{me.email}</span>
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void navigate('/profil')}>
          <UserRoundIcon className="text-indigo-500" />
          {t('nav.profile')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void navigate('/guvenlik')}>
          <ShieldCheckIcon className="text-emerald-500" />
          {t('nav.security')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void navigate('/bildirimler/tercihler')}>
          <BellRingIcon className="text-violet-500" />
          {t('notifications.preferences.title')}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <PaletteIcon />
            {t('theme.label')}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <ThemeItems />
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <LanguagesIcon />
            {t('language.label')}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <LanguageItems />
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={logout}>
          <LogOutIcon className="text-destructive" />
          {t('nav.logout')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
