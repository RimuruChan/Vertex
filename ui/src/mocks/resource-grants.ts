import type {
  DtoProblemGrantResponse,
  DtoContestGrantResponse,
  DtoSetAccessResponse,
  DtoUserResponse,
} from '@/generated/api/model'
import type { MockScope } from './domain-policy'
import type { MockState } from './fixtures'
import { mockActive } from './domain-policy'
import { mockUsers } from './identities'
import { MockError } from './errors'

type Grant = DtoProblemGrantResponse | DtoContestGrantResponse | DtoSetAccessResponse
export function effectiveRoles(
  grants: Grant[],
  user: DtoUserResponse | null,
  scope?: MockScope,
): string[] {
  if (!mockActive(scope, user)) return []
  const groups = scope?.domain?.groups ?? []
  return grants
    .filter(
      (grant) =>
        grant.userId === user?.id ||
        (!!grant.groupId &&
          groups.some((group) => group.id === grant.groupId && !!group.members[user!.id])),
    )
    .map((grant) => grant.role)
}

export function visibleGrants<T extends Grant>(grants: T[], scope?: MockScope): T[] {
  return grants
    .filter(
      (grant) =>
        !grant.groupId || scope?.domain?.groups?.some((group) => group.id === grant.groupId),
    )
    .map((grant) => ({
      ...grant,
      ...(grant.groupId
        ? { groupName: scope?.domain?.groups?.find((group) => group.id === grant.groupId)?.name }
        : {}),
    }))
}

export function grantMember(state: MockState, username: string) {
  const user =
    state.user?.username === username.trim()
      ? state.user
      : mockUsers.find((user) => user.username === username.trim())
  if (!user) throw new MockError(400, '目标须为本域有效成员')
  if (state.scope?.domain?.members[user.id]?.status !== 'active')
    throw new MockError(400, '目标须为本域有效成员')
  return user
}

export function updateGrant<T extends Grant>(
  state: MockState,
  grants: T[],
  ownerID: string,
  body: Record<string, unknown>,
  roles: readonly T['role'][],
  nextID: () => number,
  multipleRoles = false,
): T[] {
  const username = typeof body.username === 'string' ? body.username.trim() : '',
    groupRef = typeof body.group === 'string' ? body.group.trim() : '',
    role = String(body.role) as T['role']
  if (!!username === !!groupRef || !roles.includes(role))
    throw new MockError(400, '请选择一个用户或群组以及有效协作角色')
  let subject: Pick<Grant, 'userId' | 'username' | 'groupId' | 'groupName'>
  if (username) {
    const user = grantMember(state, username)
    if (user.id === ownerID) throw new MockError(400, 'owner 请通过所有权转让修改')
    subject = { userId: user.id, username: user.username }
  } else {
    const group = state.scope?.domain?.groups?.find(
      (group) => group.id === groupRef || group.publicId === groupRef,
    )
    if (!group) throw new MockError(400, '群组必须属于当前域')
    subject = { groupId: group.id, groupName: group.name }
  }
  const matching = (grant: T) =>
    (subject.userId ? grant.userId === subject.userId : grant.groupId === subject.groupId) &&
    (!multipleRoles || grant.role === role)
  const existing = grants.find(matching)
  return [
    ...grants.filter((grant) => !matching(grant)),
    { ...subject, id: existing?.id ?? nextID(), role } as T,
  ]
}

export function removeGrant<T extends Grant>(grants: T[], reference: string): T[] {
  if (!grants.some((grant) => String(grant.id) === reference))
    throw new MockError(404, '授权不存在')
  return grants.filter((grant) => String(grant.id) !== reference)
}
