import type {
  DtoProblemPermissions,
  DtoProblemResponse,
  DtoUserResponse,
} from '@/generated/api/model'

export const officialDomainID = '00000000-0000-4000-8000-000000000001'

export function problemPermissions(
  problem: Pick<DtoProblemResponse, 'ownerId' | 'visibility'>,
  user: DtoUserResponse | null,
): DtoProblemPermissions {
  const owner = !!user && (user.role === 'admin' || user.id === problem.ownerId)
  return {
    view: problem.visibility === 'public' || owner,
    readPackage: owner,
    edit: owner,
    publish: owner,
    manageAccess: owner,
    delete: owner,
    transfer: owner,
    copy: owner,
  }
}
