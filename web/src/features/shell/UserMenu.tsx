import { LanguagesIcon, LogOutIcon, PaletteIcon, ShieldCheckIcon, UserRoundIcon } from 'lucide-react'
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
    return <Skeleton className="size-8 rounded-full" />
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="h-10 gap-2 px-2" aria-label={t('nav.userMenu')}>
          <Avatar firstName={me.first_name} lastName={me.last_name} />
          <span className="hidden text-left text-sm leading-tight sm:block">
            <span className="block font-medium">
              {me.first_name} {me.last_name}
            </span>
            <span className="block text-xs text-muted-foreground">{me.username}</span>
          </span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuLabel className="font-normal">
          <p className="font-medium">
            {me.first_name} {me.last_name}
          </p>
          <p className="truncate text-xs text-muted-foreground">{me.email}</p>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void navigate('/profil')}>
          <UserRoundIcon />
          {t('nav.profile')}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => void navigate('/guvenlik')}>
          <ShieldCheckIcon />
          {t('nav.security')}
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
        <DropdownMenuItem onSelect={logout}>
          <LogOutIcon />
          {t('nav.logout')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
