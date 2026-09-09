import { createContext, useContext, useEffect, useState, type PropsWithChildren } from 'react'
import { useLocation } from 'react-router-dom'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useAuth } from '@/auth/AuthContext'
import type { DtoContestDetailsResponse } from '@/generated/api/model'

type ContestSpace = { ref: string; details: DtoContestDetailsResponse | null }
const Context = createContext<ContestSpace | null>(null)
export const useContestSpace = () => useContext(Context)

export function ContestProvider({ children }: PropsWithChildren) {
  const { pathname } = useLocation()
  const ref = pathname.match(/^\/d\/[^/]+\/contests\/([^/]+)/)?.[1] ?? ''
  const { user } = useAuth()
  const { getApiContestsId: getContest } = useDomainAPI()
  const [loaded, setLoaded] = useState<{ key: string; details: DtoContestDetailsResponse } | null>(
    null,
  )
  const key = `${ref}:${user?.id ?? ''}`
  useEffect(() => {
    if (!ref) return
    const controller = new AbortController()
    const refresh = () =>
      getContest(ref, { signal: controller.signal })
        .then((details) => {
          if (!controller.signal.aborted) setLoaded({ key, details })
        })
        .catch(() => {
          if (!controller.signal.aborted) setLoaded(null)
        })
    void refresh()
    const timer = window.setInterval(refresh, 15000)
    return () => {
      controller.abort()
      window.clearInterval(timer)
    }
  }, [key, ref, getContest])
  return (
    <Context.Provider
      value={ref ? { ref, details: loaded?.key === key ? loaded.details : null } : null}
    >
      {children}
    </Context.Provider>
  )
}
