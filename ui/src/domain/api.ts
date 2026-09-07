import * as generated from '@/generated/api/vertex'
import { validDomain } from './paths'

type Generated = typeof generated
export type DomainAPI = {
  [
    K in keyof Generated as K extends `${infer Verb}ApiDomainsDomain${infer Suffix}`
      ? `${Verb}Api${Suffix}`
      : never
  ]: Generated[K] extends (domain: string, ...args: infer A) => infer R ? (...args: A) => R : never
}

// Capture the domain in each bound function, including requests retried after
// token refresh. No mutable global "current domain" can redirect an old request.
export function bindDomainAPI(domain: string): DomainAPI {
  if (!validDomain(domain)) throw new Error('Invalid domain slug')
  const bound = Object.entries(generated).flatMap(([name, call]) => {
    const match = /^(get|post|put|patch|delete)ApiDomainsDomain(.*)$/.exec(name)
    if (!match) return []
    return [
      [
        `${match[1]}Api${match[2]}`,
        (...args: unknown[]) => (call as (...args: unknown[]) => unknown)(domain, ...args),
      ],
    ]
  })
  return Object.fromEntries(bound) as DomainAPI
}
