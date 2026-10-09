import { createContext, useContext } from 'react'

export type Theme = 'light' | 'dark' | 'system'

/** THEME_STORAGE_KEY, public/theme-init.js ile aynı olmalı. */
export const THEME_STORAGE_KEY = 'agora.theme'

export type ThemeState = {
  theme: Theme
  /** resolved, sistem teması çözülmüş halidir: ekranda gerçekten görünen tema. */
  resolved: 'light' | 'dark'
  setTheme: (theme: Theme) => void
}

export const ThemeContext = createContext<ThemeState | null>(null)

export function useTheme(): ThemeState {
  const ctx = useContext(ThemeContext)
  if (!ctx) {
    throw new Error('useTheme, ThemeProvider içinde kullanılmalı')
  }
  return ctx
}
