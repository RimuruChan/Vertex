import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { getApiDomains } from '@/generated/api/vertex'
import type { DtoDomainResponse } from '@/generated/api/model'
import { useDomain } from './DomainContext'
import { switchDomainPath } from './paths'
import { apiError } from '@/lib/format'
import { useAuth } from '@/auth/AuthContext'
import { Link } from './navigation'
import { domainRoleLabel } from './labels'

export default function DomainSwitcher() {
  const { slug, domain } = useDomain()
  const { user } = useAuth()
  const role =
    user?.role === 'admin'
      ? '站点维护'
      : domain.ownerId === user?.id && user
        ? '域 owner'
        : !user
          ? '访客'
          : domainRoleLabel(domain.memberRole)
  const location = useLocation(),
    navigate = useNavigate()
  const [items, setItems] = useState<DtoDomainResponse[]>([])
  const [error, setError] = useState<string | null>(null)
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setError(null)
    getApiDomains({ size: 100 }, { signal: controller.signal })
      .then((result) => {
        if (!controller.signal.aborted) setItems(result.items)
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(apiError(cause, '域列表加载失败'))
      })
    return () => controller.abort()
  }, [slug, retry])
  return (
    <div className="min-w-0 max-w-36 shrink sm:max-w-44">
      <label className="sr-only" htmlFor="active-domain">
        当前域
      </label>
      <select
        id="active-domain"
        className="h-9 w-full truncate rounded-md border border-input bg-background px-2 text-sm"
        value={slug}
        onChange={(e) => navigate(switchDomainPath(e.target.value, location.pathname))}
      >
        {!items.some((item) => item.slug === slug) && <option value={slug}>{domain.name}</option>}
        {items
          .filter((item) => item.canEnter)
          .map((item) => (
            <option key={item.id} value={item.slug}>
              {item.name}
              {item.archived ? '（已归档）' : ''}
            </option>
          ))}
      </select>
      <span className="ml-1 text-xs text-muted-foreground">{role}</span>
      <Link to="/domains" className="ml-2 text-xs text-primary">
        浏览域
      </Link>
      {error && (
        <button
          type="button"
          className="text-xs text-destructive"
          title={error}
          onClick={() => setRetry((value) => value + 1)}
        >
          域列表加载失败，点击重试
        </button>
      )}
    </div>
  )
}
