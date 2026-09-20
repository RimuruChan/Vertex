import { createContext, useContext } from 'react'
import type { DtoDomainResponse, DomainPermission } from '@/generated/api/model'

type DomainContextValue = {
  slug: string
  domain: DtoDomainResponse
  can: (permission: DomainPermission) => boolean
  refresh: () => Promise<void>
}

// Keep the context identity outside the provider's UI/dependency graph. Editing
// shared display helpers must not split providers and consumers during HMR.
export const DomainContext = createContext<DomainContextValue | null>(null)
export const useOptionalDomain = () => useContext(DomainContext)
export function useDomain() {
  const value = useOptionalDomain()
  if (!value) throw new Error('Domain context is required')
  return value
}
