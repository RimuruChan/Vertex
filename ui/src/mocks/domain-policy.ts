import type { DomainPermission, DtoUserResponse, DtoRoleResponse } from '@/generated/api/model'

export type MockScope = {
  id: string
  slug: string
  archived: boolean
  active: boolean
  manager: boolean
  permissions: DomainPermission[]
}
export type MockDomain = {
  id: string
  slug: string
  name: string
  description: string
  visibility: 'public' | 'private'
  archived: boolean
  ownerId?: string
  members: Record<string, { role: string; status: string; joinedAt?: string }>
  joinPolicy?: 'open' | 'approval' | 'invite'
  roles?: DtoRoleResponse[]
  groups?: MockGroup[]
  nextGroupNumber?: number
  createdAt?: string
}

export type MockGroup = {
  id: string
  publicId: string
  name: string
  description: string
  ownerId: string
  members: Record<string, 'member' | 'manager'>
  createdAt: string
}

export const domainPermissions: DomainPermission[] = [
  'domain.settings.manage',
  'domain.members.manage',
  'domain.roles.manage',
  'domain.groups.manage',
  'domain.resources.manage',
  'problem.create',
  'contest.create',
  'problem_set.create',
  'submission.create',
  'content.create',
]
export const rolePermissions: Record<string, DomainPermission[]> = {
  admin: domainPermissions,
  author: [
    'problem.create',
    'contest.create',
    'problem_set.create',
    'submission.create',
    'content.create',
  ],
  member: ['problem_set.create', 'submission.create', 'content.create'],
  viewer: [],
}
export const mockActive = (scope: MockScope | undefined, user: DtoUserResponse | null) =>
  !!user && (scope?.active ?? true)
export const mockManager = (scope: MockScope | undefined, user: DtoUserResponse | null) =>
  !!user && (scope?.manager ?? user.role === 'admin')
export const mockCan = (
  scope: MockScope | undefined,
  user: DtoUserResponse | null,
  permission: DomainPermission,
) =>
  !!user &&
  !scope?.archived &&
  (scope
    ? scope.permissions.includes(permission)
    : user.role === 'admin' || rolePermissions.member.includes(permission))
