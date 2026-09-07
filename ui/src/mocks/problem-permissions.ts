import type {
  DtoProblemPermissions,
  DtoProblemResponse,
  DtoUserResponse,
} from '@/generated/api/model'
import { mockActive, mockManager, type MockScope } from './domain-policy'

export const officialDomainID = '00000000-0000-4000-8000-000000000001'

export function problemPermissions(
  problem: Pick<DtoProblemResponse, 'ownerId' | 'visibility'> & { publishedVersion?: number },
  user: DtoUserResponse | null,
  scope?: MockScope,
): DtoProblemPermissions {
  const owner =
    mockManager(scope, user) || (mockActive(scope, user) && user?.id === problem.ownerId)
  const write = owner && !scope?.archived
  return {
    view: (problem.visibility === 'public' && problem.publishedVersion !== 0) || owner,
    readPackage: owner,
    edit: write,
    publish: write,
    manageAccess: write,
    delete: write,
    transfer: write,
    copy: owner,
  }
}
