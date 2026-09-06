import type {
  DtoSetAccessResponse,
  DtoSetPermissions,
  DtoSetResponse,
  DtoUserResponse,
} from '@/generated/api/model'

export function setPermissions(
  set: Pick<DtoSetResponse, 'ownerId' | 'visibility'>,
  user: DtoUserResponse | null,
  grants: DtoSetAccessResponse[] = [],
  allItemsVisible = true,
): DtoSetPermissions {
  const manage = !!user && (set.ownerId === user.id || user.role === 'admin')
  const roles = grants.filter((g) => !!user && g.userId === user.id).map((g) => g.role)
  const edit = manage || roles.includes('editor')
  return {
    view: set.visibility === 'public' || manage || roles.length > 0,
    viewAccess: manage || roles.length > 0,
    edit,
    editItems: edit && allItemsVisible,
    publish: manage,
    manageAccess: manage,
    delete: manage,
    transfer: manage,
  }
}
