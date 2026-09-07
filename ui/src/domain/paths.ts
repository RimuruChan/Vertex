export const defaultDomain = 'official'
export const validDomain = (slug: string) => /^[a-z][a-z0-9-]{1,31}$/.test(slug)

export function domainPath(slug: string, path = '/') {
  if (!validDomain(slug)) throw new Error('Invalid domain slug')
  if (!path.startsWith('/') || path.startsWith('//') || /^\/d\//.test(path)) return path
  const [pathname] = path.split(/[?#]/)
  if (pathname === '/admin/problems') path = path.replace('/admin/problems', '/authoring')
  else if (/^\/admin\/problems\/[^/]+\/package$/.test(pathname))
    path = path.replace(/^\/admin\/problems\/([^/]+)\/package/, '/authoring/$1')
  else if (pathname === '/admin/contests')
    path = path.replace('/admin/contests', '/manage/contests')
  if (
    !/^\/(?:$|[?#]|problems(?:\/|[?#]|$)|problem-sets(?:\/|[?#]|$)|contests(?:\/|[?#]|$)|submissions(?:\/|[?#]|$)|editorials(?:\/|[?#]|$)|users(?:\/|[?#]|$)|authoring(?:\/|[?#]|$)|manage(?:\/|[?#]|$)|groups(?:\/|[?#]|$)|settings(?:\/|[?#]|$))/.test(
      path,
    )
  )
    return path
  return `/d/${slug}${path === '/' ? '' : path}`
}

export function relativeDomainPath(pathname: string) {
  return pathname.replace(/^\/d\/[^/]+(?=\/|$)/, '') || '/'
}

export function domainReturnState(slug: string, state: unknown): unknown {
  if (!state || typeof state !== 'object' || !('from' in state) || typeof state.from !== 'string')
    return state
  return { ...state, from: domainPath(slug, state.from) }
}

export function switchDomainPath(slug: string, pathname: string) {
  const section = relativeDomainPath(pathname).split('/')[1]
  const retained = [
    'problems',
    'problem-sets',
    'contests',
    'submissions',
    'editorials',
    'authoring',
    'groups',
  ].includes(section)
  return domainPath(slug, retained ? `/${section}` : '/')
}
