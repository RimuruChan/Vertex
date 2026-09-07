import type { DtoDomainResponse, DtoUserResponse } from '@/generated/api/model'
import { createFixtures, type MockState } from './fixtures'
import {
  adminUser,
  demoUser,
  juryUser,
  observerUser,
  contestantUser,
  mockUsers,
} from './identities'
import { officialDomainID } from './problem-permissions'
import {
  domainPermissions,
  rolePermissions,
  type MockDomain,
  type MockScope,
} from './domain-policy'

export function initializeDomains(state: MockState) {
  state.domainSpaces ??= {}
  state.domains ??= [
    {
      id: officialDomainID,
      slug: 'official',
      name: '官方',
      description: 'Vertex 官方题库',
      visibility: 'public',
      archived: false,
      members: Object.fromEntries(
        mockUsers.map((user) => [user.id, { role: 'member', status: 'active' }]),
      ),
    },
    {
      id: '00000000-0000-4000-8000-000000000002',
      slug: 'training',
      name: '算法训练营',
      description: '协作出题与训练的独立空间',
      visibility: 'public',
      archived: false,
      ownerId: juryUser.id,
      members: {
        [demoUser.id]: { role: 'author', status: 'active' },
        [juryUser.id]: { role: 'admin', status: 'active' },
        [contestantUser.id]: { role: 'member', status: 'active' },
        [observerUser.id]: { role: 'viewer', status: 'active' },
      },
    },
    {
      id: '00000000-0000-4000-8000-000000000003',
      slug: 'private-team',
      name: '命题小组',
      description: '仅受邀成员可访问',
      visibility: 'private',
      archived: false,
      ownerId: juryUser.id,
      members: {
        [juryUser.id]: { role: 'admin', status: 'active' },
        [observerUser.id]: { role: 'viewer', status: 'active' },
      },
    },
  ]
  for (const domain of state.domains) {
    domain.joinPolicy ??= domain.visibility === 'private' ? 'invite' : 'open'
    domain.createdAt ??= '2026-09-01T00:00:00Z'
    domain.roles ??= Object.entries(rolePermissions).map(([key, permissions]) => ({
      key,
      name: (
        { admin: '域管理员', author: '出题人', member: '成员', viewer: '只读成员' } as Record<
          string,
          string
        >
      )[key],
      permissions: [...permissions],
      builtin: true,
    }))
    domain.groups ??=
      domain.slug === 'training'
        ? [
            {
              id: '10000000-0000-4000-8000-000000000002',
              publicId: '1',
              name: '命题协作组',
              description: '组管理者维护本组成员，不自动获得域治理权。',
              ownerId: juryUser.id,
              members: {
                [juryUser.id]: 'manager',
                [demoUser.id]: 'manager',
                [observerUser.id]: 'member',
              },
              createdAt: domain.createdAt,
            },
          ]
        : []
    domain.nextGroupNumber ??= Math.max(
      1,
      ...domain.groups.map((group) => Number(group.publicId) + 1),
    )
  }
}

export function domainView(domain: MockDomain, user: DtoUserResponse | null): DtoDomainResponse {
  const member = user ? domain.members[user.id] : undefined
  const active = member?.status === 'active'
  const owner = active && domain.ownerId === user?.id
  const admin = user?.role === 'admin'
  const permissions =
    admin || owner
      ? domainPermissions
      : active
        ? (domain.roles?.find((role) => role.key === member.role)?.permissions ??
          rolePermissions[member.role] ??
          [])
        : []
  return {
    id: domain.id,
    slug: domain.slug,
    name: domain.name,
    description: domain.description,
    ownerId: domain.ownerId,
    ownerName: mockUsers.find((u) => u.id === domain.ownerId)?.username ?? '',
    official: domain.slug === 'official',
    visibility: domain.visibility,
    joinPolicy: domain.joinPolicy ?? (domain.visibility === 'private' ? 'invite' : 'open'),
    archived: domain.archived,
    memberRole: member?.role ?? '',
    memberStatus: member?.status ?? '',
    canEnter:
      admin || (member?.status !== 'suspended' && (active || domain.visibility === 'public')),
    canTransfer: domain.slug !== 'official' && (admin || owner) && !domain.archived,
    canArchive: domain.slug !== 'official' && (admin || owner),
    permissions: domain.archived ? [] : [...permissions],
    createdAt: domain.createdAt ?? '2026-09-01T00:00:00Z',
  }
}

export function scopeFor(domain: MockDomain, user: DtoUserResponse | null): MockScope {
  const view = domainView(domain, user)
  return {
    id: domain.id,
    slug: domain.slug,
    archived: domain.archived,
    domain,
    active: view.memberStatus === 'active',
    permissions: view.permissions,
    manager:
      user?.role === 'admin' ||
      (view.memberStatus === 'active' &&
        (domain.ownerId === user?.id ||
          (
            domain.roles?.find((role) => role.key === view.memberRole)?.permissions ??
            rolePermissions[view.memberRole] ??
            []
          ).includes('domain.resources.manage'))),
  }
}

export function createDomainSpace(domain: MockDomain, now: number): MockState {
  if (!['training', 'private-team'].includes(domain.slug)) {
    const state: MockState = createFixtures(now)
    state.problems = []
    state.submissions = []
    state.contests = []
    state.editorials = []
    state.discussions = []
    state.sets = []
    state.staff = {}
    state.registrations = {}
    state.nextDiscussionId = 0
    return state
  }
  const userIDs = new Set(mockUsers.map((u) => u.id))
  const suffix = domain.id.slice(-12)
  const remap = (value: unknown): unknown => {
    if (typeof value === 'string')
      return /^[0-9a-f]{8}-[0-9a-f-]{27}$/i.test(value) && !userIDs.has(value)
        ? value.slice(0, -12) + suffix
        : value
    if (Array.isArray(value)) return value.map(remap)
    if (value && typeof value === 'object')
      return Object.fromEntries(
        Object.entries(value).map(([key, item]) => [remap(key) as string, remap(item)]),
      )
    return value
  }
  const state = remap(createFixtures(now)) as MockState
  const setter = domain.slug === 'training' ? demoUser : juryUser
  for (const problem of state.problems) {
    problem.domainId = domain.id
    problem.ownerId = setter.id
    problem.authorId = setter.id
    problem.title = `${domain.name} · ${problem.title}`
  }
  for (const contest of state.contests) {
    contest.domainId = domain.id
    contest.ownerId = setter.id
    contest.title = `${domain.name} · ${contest.title}`
  }
  for (const set of state.sets) {
    set.domainId = domain.id
    set.ownerId = setter.id
    set.title = `${domain.name} · ${set.title}`
  }
  for (const editorial of state.editorials) editorial.domainId = domain.id
  for (const post of state.discussions) post.domainId = domain.id
  state.user = { ...adminUser }
  return state
}
