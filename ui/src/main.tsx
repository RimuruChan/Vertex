import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import RootRoutes from './RootRoutes'
import { AuthProvider } from './auth/AuthContext'
import { ThemeProvider } from './components/ThemeProvider'
import { Scrollbars } from './components/Scrollbars'
import { FormValidation } from './components/FormValidation'
import { ToastProvider } from './components/ui/toast'
import { TooltipProvider } from './components/ui/misc'
import { ConfirmProvider } from './components/ui/confirm-dialog'
import './index.css'

async function bootstrap() {
  // Install before AuthProvider restores a session. Mock requests never reach the server.
  if (import.meta.env.VITE_MOCK === 'true') {
    const { installMock } = await import('./mocks/adapter')
    installMock()
  }
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <Scrollbars />
      <ThemeProvider>
        <ToastProvider>
          <FormValidation />
          <ConfirmProvider>
            <TooltipProvider delayDuration={200}>
              <BrowserRouter>
                <AuthProvider>
                  <RootRoutes />
                </AuthProvider>
              </BrowserRouter>
            </TooltipProvider>
          </ConfirmProvider>
        </ToastProvider>
      </ThemeProvider>
    </StrictMode>,
  )
}

void bootstrap()
