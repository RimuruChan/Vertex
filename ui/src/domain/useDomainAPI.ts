import { useMemo } from 'react'
import { useDomain } from './DomainContext'
import { bindDomainAPI } from './api'

export function useDomainAPI() {
  const { slug } = useDomain()
  return useMemo(() => bindDomainAPI(slug), [slug])
}
