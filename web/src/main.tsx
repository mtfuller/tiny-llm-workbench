import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import App from './App.tsx'
import { ConfirmProvider } from './ConfirmDialog.tsx'
import { EventStreamProvider } from './eventStream.tsx'
import { ToastProvider } from './Toast.tsx'
import { applyTheme, getStoredTheme } from './theme.ts'

// Applied before the first render so there's no flash of the wrong theme.
applyTheme(getStoredTheme())

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <EventStreamProvider>
        <ToastProvider>
          <ConfirmProvider>
            <App />
          </ConfirmProvider>
        </ToastProvider>
      </EventStreamProvider>
    </BrowserRouter>
  </StrictMode>,
)
