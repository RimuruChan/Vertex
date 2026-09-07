import type { DomainPermission } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockDomain } from './domain-policy'
import { domainView } from './domains'
import { mockUsers } from './identities'
import { MockError } from './errors'

export function pageItems<T>(items: T[], params: Record<string, unknown>) {
  const page = Math.max(1, Number(params.page) || 1),
    size = Math.max(1, Math.min(100, Number(params.size) || 50))
  return { items: items.slice((page - 1) * size, page * size), total: items.length }
}
export function username(state: MockState, id: string) {
  return (
    (state.user?.id === id ? state.user : mockUsers.find((user) => user.id === id))?.username ?? ''
  )
}
export function findUser(state: MockState, name: string) {
  const user =
    state.user?.username === name ? state.user : mockUsers.find((user) => user.username === name)
  if (!user) throw new MockError(404, '账号不存在')
  return user
}
export function assertDomainPermission(
  state: MockState,
  domain: MockDomain,
  permission: DomainPermission,
) {
  if (!domainView(domain, state.user).permissions.includes(permission))
    throw new MockError(403, '没有执行此操作的域权限')
}
