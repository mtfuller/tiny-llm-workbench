import { useEffect, useState } from 'react'
import { getSystemInfo, type SystemInfo } from '../api'
import Skeleton from '../Skeleton'
import { getStoredTheme, setTheme, type ThemePreference } from '../theme'

const THEME_OPTIONS: { value: ThemePreference; label: string }[] = [
  { value: 'system', label: 'Match system' },
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
]

function Settings() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [theme, setThemeState] = useState<ThemePreference>(getStoredTheme)

  useEffect(() => {
    getSystemInfo()
      .then(setInfo)
      .catch((err: Error) => setError(err.message))
  }, [])

  const handleThemeChange = (value: ThemePreference) => {
    setTheme(value)
    setThemeState(value)
  }

  return (
    <>
      <div className="page-header">
        <h2>Settings</h2>
      </div>

      <section className="panel">
        <h3>Appearance</h3>
        <p className="hint">Choose how TLW looks, or follow your system's light/dark setting.</p>
        <div className="theme-picker">
          {THEME_OPTIONS.map((option) => (
            <label key={option.value} className="checkbox-label">
              <input
                type="radio"
                name="theme"
                value={option.value}
                checked={theme === option.value}
                onChange={() => handleThemeChange(option.value)}
              />
              {option.label}
            </label>
          ))}
        </div>
      </section>

      <section className="panel">
        <h3>System</h3>
        {error && <p className="error">{error}</p>}
        {!error && info === null && (
          <dl className="info-list">
            <dt>Version</dt>
            <dd>
              <Skeleton width="6rem" />
            </dd>
            <dt>Registry root</dt>
            <dd>
              <Skeleton width="14rem" />
            </dd>
          </dl>
        )}
        {info !== null && (
          <dl className="info-list">
            <dt>Version</dt>
            <dd>
              <code>{info.version}</code>
            </dd>
            <dt>Registry root</dt>
            <dd>
              <code>{info.registryRoot}</code>
            </dd>
          </dl>
        )}
        <p className="hint">
          The registry root is set when <code>tlw serve</code> starts (via the <code>TLW_HOME</code>{' '}
          environment variable) — restart the server to change it. Training and running models both
          shell out directly to mlx-lm's CLI commands (<code>mlx_lm.lora</code>, <code>mlx_lm.server</code>),
          so they must be on PATH for whatever environment <code>tlw serve</code> runs in — install with{' '}
          <code>pip install mlx-lm</code> or <code>brew install mlx-lm</code>. If a run fails with{' '}
          <code>not found on PATH</code>, that's what's missing.
        </p>
      </section>
    </>
  )
}

export default Settings
