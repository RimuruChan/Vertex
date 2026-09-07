import type { DtoGroupResponse } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import type { MockDomain, MockGroup } from './domain-policy'
import { domainView } from './domains'
import { assertDomainPermission, findUser, pageItems, username } from './governance-helpers'
import { MockError } from './errors'

function groupView(state: MockState, domain: MockDomain, group: MockGroup): DtoGroupResponse {
  const scope = domainView(domain, state.user),
    active = scope.memberStatus === 'active',
    id = state.user?.id ?? '',
    role = active ? (group.members[id] ?? '') : ''
  const governor = scope.permissions.includes('domain.groups.manage'),
    owner = active && group.ownerId === id,
    canManage = !domain.archived && (governor || owner || role === 'manager')
  return {
    id: group.id,
    publicId: group.publicId,
    domainId: domain.id,
    name: group.name,
    description: group.description,
    ownerId: group.ownerId,
    ownerName: username(state, group.ownerId),
    memberCount: Object.keys(group.members).filter((id) => domain.members[id]?.status === 'active')
      .length,
    viewerRole: role,
    canManage,
    canTransfer: canManage && (governor || owner),
    canDelete: canManage && (governor || owner),
    createdAt: group.createdAt,
  }
}

export function groupRequest(
  state: MockState,
  domain: MockDomain,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
) {
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent),
    ref = parts[4],
    action = parts[5],
    name = parts[6],
    get = method === 'GET',
    iso = new Date(now).toISOString()
  if (parts.length > 7 || (action === 'owner' && parts.length !== 6))
    throw new MockError(404, '群组接口不存在')
  const text = (key: string) => (typeof body[key] === 'string' ? (body[key] as string).trim() : '')
  const validDetails = () => {
    if (!text('name') || [...text('name')].length > 100 || text('description').length > 8000)
      throw new MockError(400, '群组设置不合法')
  }
  if (!ref && get) {
    const query = String(params.keyword ?? '').toLowerCase()
    return pageItems(
      domain
        .groups!.filter((group) => group.name.toLowerCase().includes(query))
        .map((group) => groupView(state, domain, group))
        .sort((a, b) => a.name.localeCompare(b.name)),
      params,
    )
  }
  if (!ref && method === 'POST') {
    assertDomainPermission(state, domain, 'domain.groups.manage')
    validDetails()
    const owner = findUser(state, text('ownerUsername') || state.user!.username)
    if (domain.members[owner.id]?.status !== 'active')
      throw new MockError(400, '组所有者必须是有效域成员')
    const group: MockGroup = {
      id: crypto.randomUUID(),
      publicId: String(domain.nextGroupNumber!++),
      name: text('name'),
      description: text('description'),
      ownerId: owner.id,
      members: { [owner.id]: 'manager' },
      createdAt: iso,
    }
    domain.groups!.push(group)
    return groupView(state, domain, group)
  }
  const group = domain.groups!.find((group) => group.id === ref || group.publicId === ref)
  if (!group) throw new MockError(404, '群组不存在')
  const view = groupView(state, domain, group)
  if (get && !action) return view
  if (get && action === 'members') {
    if (!view.canManage && !view.viewerRole)
      throw new MockError(403, '只有组成员和管理者可查看成员')
    const query = String(params.keyword ?? '').toLowerCase()
    return pageItems(
      Object.entries(group.members)
        .filter(([id]) => domain.members[id]?.status === 'active')
        .map(([id, role]) => ({ userId: id, username: username(state, id), role }))
        .filter((member) => member.username.toLowerCase().includes(query))
        .sort((a, b) => a.username.localeCompare(b.username)),
      params,
    )
  }
  if (!view.canManage) throw new MockError(403, '没有管理这个组的权限')
  if (method === 'PUT' && !action) {
    validDetails()
    group.name = text('name')
    group.description = text('description')
    return { status: 'ok' }
  }
  if (method === 'DELETE' && !action) {
    if (!view.canDelete) throw new MockError(403, '组管理者不能删除组')
    domain.groups = domain.groups!.filter((g) => g.id !== group.id)
    return { status: 'ok' }
  }
  if (method === 'PUT' && action === 'owner') {
    if (!view.canTransfer) throw new MockError(403, '组管理者不能转让所有权')
    const target = findUser(state, text('username'))
    if (domain.members[target.id]?.status !== 'active')
      throw new MockError(400, '新所有者必须是有效域成员')
    group.ownerId = target.id
    group.members[target.id] = 'manager'
    return { status: 'ok' }
  }
  if (action === 'members' && name) {
    const target = findUser(state, name),
      member = domain.members[target.id],
      remove = method === 'DELETE',
      role = text('role')
    if (!remove && method !== 'PUT') throw new MockError(404, '群组接口不存在')
    if (!member || (!remove && member.status !== 'active'))
      throw new MockError(400, '只能添加有效的同域成员')
    if (!remove && !['member', 'manager'].includes(role)) throw new MockError(400, '组内角色不合法')
    if (group.ownerId === target.id && (remove || role !== 'manager'))
      throw new MockError(409, '不能移除或降级组所有者')
    if (remove) delete group.members[target.id]
    else group.members[target.id] = role as 'member' | 'manager'
    return { status: 'ok' }
  }
  throw new MockError(404, '群组接口不存在')
}
