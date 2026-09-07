import { useCallback, forwardRef } from 'react'
import {
  Link as RouterLink,
  NavLink as RouterNavLink,
  useNavigate as useRouterNavigate,
  type LinkProps,
  type NavLinkProps,
  type NavigateFunction,
  type NavigateOptions,
  type To,
} from 'react-router-dom'
import { useOptionalDomain } from './DomainContext'
import { defaultDomain, domainPath, domainReturnState } from './paths'

function scopedTo(slug: string, to: To): To {
  return typeof to === 'string'
    ? domainPath(slug, to)
    : { ...to, ...(to.pathname ? { pathname: domainPath(slug, to.pathname) } : {}) }
}
export const Link = forwardRef<HTMLAnchorElement, LinkProps>(function DomainLink(
  { to, ...props },
  ref,
) {
  const slug = useOptionalDomain()?.slug ?? defaultDomain
  return (
    <RouterLink
      {...props}
      state={domainReturnState(slug, props.state)}
      ref={ref}
      to={scopedTo(slug, to)}
    />
  )
})
export const NavLink = forwardRef<HTMLAnchorElement, NavLinkProps>(function DomainNavLink(
  { to, ...props },
  ref,
) {
  const slug = useOptionalDomain()?.slug ?? defaultDomain
  return (
    <RouterNavLink
      {...props}
      state={domainReturnState(slug, props.state)}
      ref={ref}
      to={scopedTo(slug, to)}
    />
  )
})
export function useNavigate(): NavigateFunction {
  const native = useRouterNavigate()
  const slug = useOptionalDomain()?.slug ?? defaultDomain
  return useCallback(
    ((to: To | number, options?: NavigateOptions) =>
      typeof to === 'number'
        ? native(to)
        : native(scopedTo(slug, to), {
            ...options,
            state: domainReturnState(slug, options?.state),
          })) as NavigateFunction,
    [native, slug],
  )
}
