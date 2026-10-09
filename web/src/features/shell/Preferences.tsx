import { LanguagesIcon, MonitorIcon, MoonIcon, SunIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useTheme, type Theme } from '@/app/theme-context'
import { languages } from '@/i18n'
import { Button } from '@/shared/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'

const themeIcons = { light: SunIcon, dark: MoonIcon, system: MonitorIcon } as const
const themes: Theme[] = ['light', 'dark', 'system']

/** ThemeItems, tema seçeneklerini bir menü içinde radyo öğeleri olarak gösterir. */
export function ThemeItems() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  return (
    <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
      {themes.map((th) => {
        const Icon = themeIcons[th]
        return (
          <DropdownMenuRadioItem key={th} value={th}>
            <Icon />
            {t(`theme.${th}`)}
          </DropdownMenuRadioItem>
        )
      })}
    </DropdownMenuRadioGroup>
  )
}

export function LanguageItems() {
  const { t, i18n } = useTranslation()
  return (
    <DropdownMenuRadioGroup value={i18n.language} onValueChange={(v) => void i18n.changeLanguage(v)}>
      {languages.map((lng) => (
        <DropdownMenuRadioItem key={lng} value={lng} lang={lng}>
          {t(`language.${lng}`)}
        </DropdownMenuRadioItem>
      ))}
    </DropdownMenuRadioGroup>
  )
}

/** PreferenceButtons, giriş ekranlarındaki tema ve dil seçicileridir. */
export function PreferenceButtons() {
  const { t } = useTranslation()
  const { resolved } = useTheme()
  const ThemeIcon = resolved === 'dark' ? MoonIcon : SunIcon
  return (
    <div className="flex items-center gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" aria-label={t('language.label')}>
            <LanguagesIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>{t('language.label')}</DropdownMenuLabel>
          <LanguageItems />
        </DropdownMenuContent>
      </DropdownMenu>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" aria-label={t('theme.label')}>
            <ThemeIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>{t('theme.label')}</DropdownMenuLabel>
          <ThemeItems />
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
