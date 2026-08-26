export type ThemePreference = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'tlw-theme'

export function getStoredTheme(): ThemePreference {
  const stored = localStorage.getItem(STORAGE_KEY)
  return stored === 'light' || stored === 'dark' ? stored : 'system'
}

// applyTheme sets (or clears) the data-theme attribute index.css keys its
// manual light/dark overrides off. Called once at module load (before React
// mounts, so there's no flash of the wrong theme) and again whenever the
// user changes the setting.
export function applyTheme(theme: ThemePreference): void {
  if (theme === 'system') {
    document.documentElement.removeAttribute('data-theme')
  } else {
    document.documentElement.setAttribute('data-theme', theme)
  }
}

export function setTheme(theme: ThemePreference): void {
  localStorage.setItem(STORAGE_KEY, theme)
  applyTheme(theme)
}
