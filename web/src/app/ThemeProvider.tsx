import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'

import { THEME_STORAGE_KEY, ThemeContext, type Theme } from './theme-context'

const darkQuery = '(prefers-color-scheme: dark)'

function storedTheme(): Theme {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') {
      return v
    }
  } catch {
    // Depolama engellenmiş olabilir (gizli pencere): sistem teması kullanılır.
  }
  return 'system'
}

/** ThemeProvider, açık/koyu/sistem temasını yönetir ve seçimi hatırlar. */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(storedTheme)
  const [systemDark, setSystemDark] = useState(() => window.matchMedia(darkQuery).matches)

  useEffect(() => {
    const mq = window.matchMedia(darkQuery)
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const resolved = theme === 'system' ? (systemDark ? 'dark' : 'light') : theme

  useEffect(() => {
    document.documentElement.classList.toggle('dark', resolved === 'dark')
  }, [resolved])

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t)
    try {
      localStorage.setItem(THEME_STORAGE_KEY, t)
    } catch {
      // Seçim bu oturumla sınırlı kalır.
    }
  }, [])

  const value = useMemo(() => ({ theme, resolved, setTheme }), [theme, resolved, setTheme])
  return <ThemeContext value={value}>{children}</ThemeContext>
}
