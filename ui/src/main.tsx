import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import RootRoutes from './RootRoutes'
import { AuthProvider } from './auth/AuthContext'
import { ThemeProvider } from './components/ThemeProvider'
import { ToastProvider } from './components/ui/toast'
import { TooltipProvider } from './components/ui/misc'
import { ConfirmProvider } from './components/ui/confirm-dialog'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <ToastProvider>
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
