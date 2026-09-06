import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react'
import type { DtoUserResponse } from '../generated/api/model'
import {
  currentUser,
  login as loginRequest,
  logout as logoutRequest,
  register as registerRequest,
  restoreSession,
  subscribeSession,
} from '../api/client'

type AuthContextValue = {
  user: DtoUserResponse | null
  ready: boolean
  login: (username: string, password: string) => Promise<void>
  register: (username: string, email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<DtoUserResponse | null>(currentUser())
  const [ready, setReady] = useState(false)

  useEffect(() => {
    const unsubscribe = subscribeSession(setUser)
    void restoreSession().finally(() => setReady(true))
    return unsubscribe
  }, [])

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      ready,
      login: async (username, password) => {
        await loginRequest(username, password)
      },
      register: async (username, email, password) => {
        await registerRequest(username, email, password)
      },
      logout: logoutRequest,
    }),
    [ready, user],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used inside AuthProvider')
  return context
}
