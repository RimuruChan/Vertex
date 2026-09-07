import type { DomainPermission } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import type { MockDomain } from './domain-policy'
import { domainPermissions } from './domain-policy'
import { domainView, initializeDomains } from './domains'
import { MockError } from './errors'
import { groupRequest } from './group-governance'
import { pageItems, username, findUser, assertDomainPermission } from './governance-helpers'

export function isDomainRequest(path: string) {
  return (
    path === '/api/domain-permissions' ||
    /^\/api\/domains(?:\/[^/]+(?:\/(?:membership|owner|archive|members|roles|groups)(?:\/.*)?)?)?$/.test(
      path,
    )
  )
}
export const permissionNames: Record<DomainPermission, string> = {
  'domain.settings.manage': '管理域设置',
  'domain.members.manage': '管理成员',
  'domain.roles.manage': '管理角色',
  'domain.groups.manage': '管理群组',
  'domain.resources.manage': '管理全域资源',
  'problem.create': '创建题目',
  'contest.create': '创建比赛',
  'problem_set.create': '创建题单',
  'submission.create': '提交程序',
  'content.create': '发布题解和讨论',
}
const nameValid = (name: string) => !!name.trim() && [...name.trim()].length <= 100

export function domainRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
): unknown {
  initializeDomains(state)
  const [, resource, slug, action, key] = path.split('/').filter(Boolean).map(decodeURIComponent)
  const count = path.split('/').filter(Boolean).length
  if (
    action &&
    ((['membership', 'owner', 'archive'].includes(action) && count !== 4) ||
      (['members', 'roles'].includes(action) && count > 5))
  )
    throw new MockError(404, '域接口不存在')
  const get = method === 'GET',
    user = state.user,
    iso = new Date(now).toISOString()
  const text = (key: string) => (typeof body[key] === 'string' ? (body[key] as string).trim() : '')
  if (resource === 'domain-permissions' && get)
    return {
      items: domainPermissions.map((key) => ({ key, label: permissionNames[key] })),
      total: domainPermissions.length,
    }
  if (!get && !user) throw new MockError(401, '请先登录')
  if (!get && new TextEncoder().encode(JSON.stringify(body)).length > 16 * 1024)
    throw new MockError(413, '请求过大')
  const discover = (domain: MockDomain) =>
    user?.role === 'admin' ||
    domain.visibility === 'public' ||
    ['active', 'pending', 'invited'].includes(domain.members[user?.id ?? '']?.status ?? '')
  if (!slug && get) {
    const query = String(params.keyword ?? '').toLowerCase()
    return pageItems(
      state
        .domains!.filter(
          (domain) =>
            discover(domain) &&
            (!domain.archived ||
              user?.role === 'admin' ||
              domain.members[user?.id ?? '']?.status === 'active') &&
            `${domain.slug} ${domain.name}`.toLowerCase().includes(query),
        )
        .map((domain) => domainView(domain, user)),
      params,
    )
  }
  if (!slug && method === 'POST') {
    const slug = text('slug').toLowerCase(),
      name = text('name'),
      visibility = text('visibility') || 'private',
      joinPolicy = text('joinPolicy') || 'invite'
    if (
      !/^[a-z][a-z0-9-]{1,31}$/.test(slug) ||
      slug === 'official' ||
      !nameValid(name) ||
      !['public', 'private'].includes(visibility) ||
      !['open', 'approval', 'invite'].includes(joinPolicy) ||
      text('description').length > 8000
    )
      throw new MockError(400, '域设置不合法')
    if (state.domains!.some((domain) => domain.slug === slug))
      throw new MockError(409, '域标识已存在')
    const domain: MockDomain = {
      id: crypto.randomUUID(),
      slug,
      name,
      description: text('description'),
      visibility: visibility as MockDomain['visibility'],
      joinPolicy: joinPolicy as MockDomain['joinPolicy'],
      archived: false,
      ownerId: user!.id,
      members: { [user!.id]: { role: 'admin', status: 'active', joinedAt: iso } },
      createdAt: iso,
    }
    state.domains!.push(domain)
    initializeDomains(state)
    return domainView(domain, user)
  }
  const domain = state.domains!.find((domain) => domain.slug === slug)
  if (!domain || !discover(domain)) throw new MockError(404, '域不存在或不可访问')
  const view = domainView(domain, user)
  if (!action && get) return view
  if (!get && domain.archived && action !== 'archive')
    throw new MockError(403, '域已归档，只能读取')
  if (!action && method === 'PUT') {
    assertDomainPermission(state, domain, 'domain.settings.manage')
    if (
      !nameValid(text('name')) ||
      text('description').length > 8000 ||
      !['public', 'private'].includes(text('visibility')) ||
      !['open', 'approval', 'invite'].includes(text('joinPolicy'))
    )
      throw new MockError(400, '域设置不合法')
    if (slug === 'official' && (text('visibility') !== 'public' || text('joinPolicy') !== 'open'))
      throw new MockError(403, '官方域必须保持公开且开放加入')
    Object.assign(domain, {
      name: text('name'),
      description: text('description'),
      visibility: text('visibility'),
      joinPolicy: text('joinPolicy'),
    })
    return domainView(domain, user)
  }
  if (action === 'membership' && method === 'POST') {
    if (view.memberStatus === 'suspended') throw new MockError(403, '域成员已停用')
    if (view.memberStatus === 'active') return view
    if (view.memberStatus === 'invited') domain.members[user!.id].status = 'active'
    else {
      if (domain.joinPolicy === 'invite') throw new MockError(403, '此域仅允许受邀加入')
      domain.members[user!.id] = {
        role: 'member',
        status: domain.joinPolicy === 'approval' ? 'pending' : 'active',
        joinedAt: iso,
      }
    }
    return domainView(domain, user)
  }
  if (action === 'archive' && method === 'PUT') {
    if (!view.canArchive) throw new MockError(403, '不能归档或恢复这个域')
    if (typeof body.archived !== 'boolean') throw new MockError(400, '归档状态无效')
    domain.archived = body.archived
    return { status: 'ok' }
  }
  if (action === 'owner' && method === 'PUT') {
    if (!view.canTransfer) throw new MockError(403, '不能转让这个域')
    const target = findUser(state, text('username'))
    if (domain.members[target.id]?.status !== 'active')
      throw new MockError(400, '新所有者必须是有效域成员')
    domain.ownerId = target.id
    domain.members[target.id].role = 'admin'
    return { status: 'ok' }
  }
  if (!user) throw new MockError(401, '请先登录')
  if (user.role !== 'admin' && view.memberStatus !== 'active')
    throw new MockError(403, '需要有效域成员身份')
  const delegate = (permissions: DomainPermission[]) => {
    if (permissions.some((permission) => !view.permissions.includes(permission)))
      throw new MockError(403, '不能授予自己没有的权限')
  }
  if (action === 'members') {
    if (get && !key) {
      const query = String(params.keyword ?? '').toLowerCase()
      return pageItems(
        Object.entries(domain.members)
          .map(([id, member]) => ({
            userId: id,
            username: username(state, id),
            roleKey: member.role,
            status: member.status,
            joinedAt: member.joinedAt ?? domain.createdAt!,
          }))
          .filter((member) => member.username.toLowerCase().includes(query))
          .sort((a, b) => a.username.localeCompare(b.username)),
        params,
      )
    }
    if (method === 'PUT' && key) {
      assertDomainPermission(state, domain, 'domain.members.manage')
      const target = findUser(state, key),
        role = domain.roles!.find((role) => role.key === text('roleKey')),
        status = text('status')
      if (!role) throw new MockError(404, '域角色不存在')
      if (!['active', 'pending', 'invited', 'suspended'].includes(status))
        throw new MockError(400, '成员状态不合法')
      const old = domain.members[target.id]
      if (target.id === domain.ownerId && (old?.role !== role.key || status !== 'active'))
        throw new MockError(409, '不能通过成员操作更改域所有者')
      delegate(role.permissions)
      domain.members[target.id] = { role: role.key, status, joinedAt: old?.joinedAt ?? iso }
      return { status: 'ok' }
    }
  }
  if (action === 'roles') {
    if (get && !key) return { items: domain.roles, total: domain.roles!.length }
    assertDomainPermission(state, domain, 'domain.roles.manage')
    const role = domain.roles!.find((role) => role.key === key)
    if (role?.builtin) throw new MockError(403, '内置角色不能修改或删除')
    if (method === 'PUT' && key) {
      if (
        !/^[a-z][a-z0-9_-]{0,31}$/.test(key) ||
        !nameValid(text('name')) ||
        !Array.isArray(body.permissions) ||
        body.permissions.some(
          (permission) => !domainPermissions.includes(permission as DomainPermission),
        )
      )
        throw new MockError(400, '角色或权限不合法')
      const permissions = [...new Set(body.permissions as DomainPermission[])].sort()
      delegate(permissions)
      const next = { key, name: text('name'), permissions, builtin: false }
      if (role) Object.assign(role, next)
      else domain.roles!.push(next)
      return { status: 'ok' }
    }
    if (method === 'DELETE' && role) {
      if (Object.values(domain.members).some((member) => member.role === key))
        throw new MockError(409, '角色仍有成员使用')
      domain.roles = domain.roles!.filter((role) => role.key !== key)
      return { status: 'ok' }
    }
  }
  if (action === 'groups') return groupRequest(state, domain, { method, path, params, body }, now)
  throw new MockError(404, '域接口不存在')
}
