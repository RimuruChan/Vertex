import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import { useNavigate } from '@/domain/navigation'
import { matchesReference } from '@/lib/routes'
import { useOptionalDomain } from '@/domain/DomainContext'
import { domainPath, defaultDomain } from '@/domain/paths'

export function useCanonicalPath(path: string | undefined, removeContestQuery = false) {
  const slug = useOptionalDomain()?.slug ?? defaultDomain
  const canonical = path ? domainPath(slug, path) : undefined
  const location = useLocation()
  const navigate = useNavigate()
  useEffect(() => {
    if (!canonical) return
    const query = new URLSearchParams(location.search)
    if (removeContestQuery) query.delete('contest')
    const search = query.size ? `?${query}` : ''
    if (location.pathname !== canonical || location.search !== search)
      navigate({ pathname: canonical, search, hash: location.hash }, { replace: true })
  }, [canonical, location.pathname, location.search, location.hash, removeContestQuery, navigate])
}

export function useCanonicalResourcePath(
  kind: string,
  ref: string | undefined,
  resource: { id: string; publicId?: string } | null | undefined,
) {
  useCanonicalPath(
    matchesReference(ref, resource) && resource?.publicId
      ? `/${kind}/${resource.publicId}`
      : undefined,
  )
}
