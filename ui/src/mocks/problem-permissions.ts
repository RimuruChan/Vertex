import type {
  DtoProblemPermissions,
  DtoProblemResponse,
  DtoUserResponse,
  DtoProblemGrantResponse,
} from '@/generated/api/model'
import { mockActive, mockManager, type MockScope } from './domain-policy'
import { effectiveRoles } from './resource-grants'

export const officialDomainID = '00000000-0000-4000-8000-000000000001'

export function problemPermissions(
  problem: Pick<DtoProblemResponse, 'ownerId' | 'visibility'> & { publishedVersion?: number },
  user: DtoUserResponse | null,
  scope?: MockScope,
  grants: DtoProblemGrantResponse[] = [],
): DtoProblemPermissions {
  const owner =
    mockManager(scope, user) || (mockActive(scope, user) && user?.id === problem.ownerId)
  const write = owner && !scope?.archived
  const roles = effectiveRoles(grants, user, scope)
  const readPackage = owner || roles.includes('reader') || roles.includes('editor')
  return {
    view: (problem.visibility === 'public' && problem.publishedVersion !== 0) || readPackage,
    readPackage,
    edit: !scope?.archived && (owner || roles.includes('editor')),
    publish: write,
    manageAccess: write,
    delete: write,
    transfer: write,
    copy: readPackage,
  }
}
