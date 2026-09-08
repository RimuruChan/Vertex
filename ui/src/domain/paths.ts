export const defaultDomain = 'official'
export const validDomain = (slug: string) => /^[a-z][a-z0-9-]{1,31}$/.test(slug)

export function domainPath(slug: string, path = '/') {
  if (!validDomain(slug)) throw new Error('Invalid domain slug')
  if (!path.startsWith('/') || path.startsWith('//') || /^\/d\//.test(path)) return path
  const [pathname] = path.split(/[?#]/)
  if (pathname === '/admin/problems' || pathname === '/authoring')
    path = path.replace(pathname, '/workspace/problems')
  else if (/^\/admin\/problems\/[^/]+\/package$/.test(pathname))
    path = path.replace(/^\/admin\/problems\/([^/]+)\/package/, '/authoring/$1')
  else if (pathname === '/admin/contests' || pathname === '/manage/contests')
    path = path.replace(pathname, '/workspace/contests')
  if (
    !/^\/(?:$|[?#]|problems(?:\/|[?#]|$)|problem-sets(?:\/|[?#]|$)|contests(?:\/|[?#]|$)|submissions(?:\/|[?#]|$)|announcements(?:\/|[?#]|$)|editorials(?:\/|[?#]|$)|users(?:\/|[?#]|$)|authoring(?:\/|[?#]|$)|workspace(?:\/|[?#]|$)|manage(?:\/|[?#]|$)|groups(?:\/|[?#]|$)|settings(?:\/|[?#]|$))/.test(
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
  const [, section, subsection] = relativeDomainPath(pathname).split('/')
  if (section === 'workspace')
    return domainPath(slug, `/workspace/${subsection === 'contests' ? 'contests' : 'problems'}`)
  if (section === 'manage' && subsection === 'contests')
    return domainPath(slug, '/workspace/contests')
  const retained = [
    'problems',
    'problem-sets',
    'contests',
    'submissions',
    'editorials',
    'announcements',
    'authoring',
    'groups',
  ].includes(section)
  return domainPath(slug, retained ? `/${section}` : '/')
}
