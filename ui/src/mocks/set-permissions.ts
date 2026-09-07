import type {
  DtoSetAccessResponse,
  DtoSetPermissions,
  DtoSetResponse,
  DtoUserResponse,
} from '@/generated/api/model'
import { mockActive, mockManager, type MockScope } from './domain-policy'

export function setPermissions(
  set: Pick<DtoSetResponse, 'ownerId' | 'visibility'>,
  user: DtoUserResponse | null,
  grants: DtoSetAccessResponse[] = [],
  allItemsVisible = true,
  scope?: MockScope,
): DtoSetPermissions {
  const manage = mockManager(scope, user) || (mockActive(scope, user) && set.ownerId === user?.id)
  const roles = grants
    .filter((g) => mockActive(scope, user) && g.userId === user?.id)
    .map((g) => g.role)
  const edit = !scope?.archived && (manage || roles.includes('editor'))
  return {
    view: set.visibility === 'public' || manage || roles.length > 0,
    viewAccess: manage || roles.length > 0,
    edit,
    editItems: edit && allItemsVisible,
    publish: manage && !scope?.archived,
    manageAccess: manage && !scope?.archived,
    delete: manage && !scope?.archived,
    transfer: manage && !scope?.archived,
  }
}
