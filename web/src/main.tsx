import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import App from './App.tsx'
import { EventStreamProvider } from './eventStream.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <EventStreamProvider>
        <App />
      </EventStreamProvider>
    </BrowserRouter>
  </StrictMode>,
)
