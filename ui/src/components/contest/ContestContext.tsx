import {
  createContext,
  useContext,
  useEffect,
  useState,
  useCallback,
  type PropsWithChildren,
} from 'react'
import { useLocation } from 'react-router-dom'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useAuth } from '@/auth/AuthContext'
import { useNavigate } from '@/domain/navigation'
import type { DtoContestDetailsResponse } from '@/generated/api/model'

type ContestSpace = {
  ref: string
  details: DtoContestDetailsResponse | null
  workspaceHref: string
  refresh: () => void
}
const Context = createContext<ContestSpace | null>(null)
export const useContestSpace = () => useContext(Context)

export function ContestProvider({ children }: PropsWithChildren) {
  const { pathname, search } = useLocation()
  const navigate = useNavigate()
  const ref = pathname.match(/^\/d\/[^/]+\/contests\/([^/]+)/)?.[1] ?? ''
  const { user } = useAuth()
  const { getApiContestsId: getContest } = useDomainAPI()
  const [loaded, setLoaded] = useState<{ key: string; details: DtoContestDetailsResponse } | null>(
    null,
  )
  const key = `${ref}:${user?.id ?? ''}`
  const memoryKey = `${import.meta.env.VITE_MOCK ? 'vertex-mock' : 'vertex'}:contest-workspace:${pathname.split('/')[2]}:${key}`
  const [workspace, setWorkspace] = useState<{ key: string; problem: string | null } | null>(null)
  const readRemembered = () => {
    try {
      const value = sessionStorage.getItem(memoryKey)
      return value && /^[^/?#]{1,256}$/.test(value) ? value : null
    } catch {
      return null
    }
  }
  const remembered = workspace?.key === memoryKey ? workspace.problem : readRemembered()
  const editing = pathname.match(/\/contests\/[^/]+\/problems\/([^/]+)$/)?.[1]
  const explicitList =
    pathname.endsWith(`/contests/${ref}`) && new URLSearchParams(search).get('tab') === 'problems'
  const activeProblem = editing ?? (explicitList ? null : remembered)
  const workspaceHref = activeProblem
    ? `/contests/${ref}/problems/${activeProblem}`
    : `/contests/${ref}?tab=problems`
  useEffect(() => {
    if (!ref) return
    if (editing || explicitList) {
      const problem = editing ?? null
      setWorkspace({ key: memoryKey, problem })
      try {
        if (problem) sessionStorage.setItem(memoryKey, problem)
        else sessionStorage.removeItem(memoryKey)
      } catch {
        /* Navigation remains usable without storage. */
      }
    } else if (
      pathname.endsWith(`/contests/${ref}`) &&
      !new URLSearchParams(search).has('tab') &&
      remembered
    ) {
      navigate(`/contests/${ref}/problems/${remembered}`, { replace: true })
    }
  }, [ref, editing, explicitList, memoryKey, pathname, search, remembered, navigate])
  const [revision, setRevision] = useState(0)
  const refresh = useCallback(() => setRevision((value) => value + 1), [])
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
  }, [key, ref, getContest, revision])
  return (
    <Context.Provider
      value={
        ref
          ? { ref, details: loaded?.key === key ? loaded.details : null, workspaceHref, refresh }
          : null
      }
    >
      {children}
    </Context.Provider>
  )
}
