import {
  createContext,
  useContext,
  useEffect,
  useCallback,
  useState,
  type PropsWithChildren,
  type ReactNode,
} from 'react'
import { useParams, Link } from 'react-router-dom'
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
  refresh: () => Promise<void>
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
  const refresh = useCallback(async () => {
    try {
      const value = await getApiDomainsDomain(slug)
      setDomain(value)
    } catch (cause) {
      setError(apiError(cause, '域信息已失效'))
    }
  }, [slug])
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
  if (!domain.canEnter)
    return frame(
      <div className="mx-auto max-w-xl px-4 py-20">
        <EmptyState
          title={`暂时无法进入「${domain.name}」`}
          description={
            domain.memberStatus === 'invited'
              ? '你收到了一份域邀请，请先接受邀请。'
              : domain.memberStatus === 'pending'
                ? '加入申请正在等待域管理员审核。'
                : domain.memberStatus === 'suspended'
                  ? '你的域成员资格已被停用，请联系域管理员。'
                  : '这个域需要有效成员身份。'
          }
          action={
            <Button asChild>
              <Link to="/domains">前往域目录</Link>
            </Button>
          }
        />
      </div>,
    )
  return (
    <DomainContext.Provider
      value={{
        slug,
        domain,
        can: (permission) => domain.permissions.includes(permission),
        refresh,
      }}
    >
      {children}
    </DomainContext.Provider>
  )
}
