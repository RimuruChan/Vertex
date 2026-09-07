import {
  createContext,
  useContext,
  useEffect,
  useState,
  type PropsWithChildren,
  type ReactNode,
} from 'react'
import { useParams } from 'react-router-dom'
import { getApiDomainsDomain } from '@/generated/api/vertex'
import type { DtoDomainResponse, DomainPermission } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { Button } from '@/components/ui/button'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { apiError } from '@/lib/format'
import { validDomain } from './paths'

type DomainContextValue = {
  slug: string
  domain: DtoDomainResponse
  can: (permission: DomainPermission) => boolean
}
const DomainContext = createContext<DomainContextValue | null>(null)
export const useOptionalDomain = () => useContext(DomainContext)
export function useDomain() {
  const value = useOptionalDomain()
  if (!value) throw new Error('Domain context is required')
  return value
}

export function DomainProvider({
  children,
  frame,
}: PropsWithChildren<{ frame: (content: ReactNode) => ReactNode }>) {
  const { domain: slug = '' } = useParams()
  const { user, ready } = useAuth()
  const [domain, setDomain] = useState<DtoDomainResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    setDomain(null)
    setLoading(true)
    setError(null)
    if (!validDomain(slug)) {
      setError('域不存在')
      setLoading(false)
      return
    }
    getApiDomainsDomain(slug, { signal: controller.signal })
      .then((value) => {
        if (!controller.signal.aborted) {
          if (!value.canEnter) throw new Error('你尚未获得这个域的访问资格')
          setDomain(value)
        }
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(apiError(cause, '无法打开这个域'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [slug, user?.id, ready, retry])
  if (!ready || loading) return frame(<PageSpinner />)
  if (!domain || error)
    return frame(
      <div className="mx-auto max-w-xl px-4 py-20">
        <EmptyState
          title="无法进入这个域"
          description={error ?? '域不存在或没有访问权限'}
          action={<Button onClick={() => setRetry((v) => v + 1)}>重试</Button>}
        />
        <a className="mt-4 block text-center text-sm text-primary" href="/d/official">
          返回官方域
        </a>
      </div>,
    )
  return (
    <DomainContext.Provider
      value={{ slug, domain, can: (permission) => domain.permissions.includes(permission) }}
    >
      {children}
    </DomainContext.Provider>
  )
}
