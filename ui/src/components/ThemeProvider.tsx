import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import type { PropsWithChildren } from 'react'

type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'vertex-theme'

type ThemeContextValue = {
  theme: Theme
  resolved: 'light' | 'dark'
  setTheme: (theme: Theme) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function systemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function storedTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
  } catch {
    return 'system'
  }
}

export function ThemeProvider({ children }: PropsWithChildren) {
  const [theme, setThemeState] = useState<Theme>(storedTheme)
  const [resolved, setResolved] = useState<'light' | 'dark'>(() =>
    storedTheme() === 'system' ? systemTheme() : (storedTheme() as 'light' | 'dark'),
  )

  useEffect(() => {
    const apply = () => {
      const next = theme === 'system' ? systemTheme() : theme
      setResolved(next)
      document.documentElement.classList.toggle('dark', next === 'dark')
    }
    apply()

    if (theme !== 'system') return
    // Only follow the OS while the user has not pinned a theme.
    const query = window.matchMedia('(prefers-color-scheme: dark)')
    query.addEventListener('change', apply)
    return () => query.removeEventListener('change', apply)
  }, [theme])

  const value = useMemo<ThemeContextValue>(
    () => ({
      theme,
      resolved,
      setTheme: (next) => {
        try {
          localStorage.setItem(STORAGE_KEY, next)
        } catch {
          /* Still apply the theme for this visit. */
        }
        setThemeState(next)
      },
    }),
    [resolved, theme],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  const context = useContext(ThemeContext)
  if (!context) throw new Error('useTheme must be used inside ThemeProvider')
  return context
}
