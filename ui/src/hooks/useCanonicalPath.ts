import { useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { matchesReference } from '@/lib/routes'

export function useCanonicalPath(path: string | undefined, removeContestQuery = false) {
  const location = useLocation()
  const navigate = useNavigate()
  useEffect(() => {
    if (!path) return
    const query = new URLSearchParams(location.search)
    if (removeContestQuery) query.delete('contest')
    const search = query.size ? `?${query}` : ''
    if (location.pathname !== path || location.search !== search)
      navigate({ pathname: path, search }, { replace: true })
  }, [path, location.pathname, location.search, removeContestQuery, navigate])
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
