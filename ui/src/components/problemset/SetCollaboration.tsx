import type { DtoSetResponse } from '@/generated/api/model'
import ResourceCollaboration from '@/components/ResourceCollaboration'

export default function SetCollaboration({
  set,
  onTransferred,
}: {
  set: DtoSetResponse
  onTransferred: () => void
}) {
  return (
    <ResourceCollaboration
      kind="set"
      id={set.id}
      ownerId={set.ownerId}
      ownerName={set.ownerName}
      manage={set.permissions.manageAccess}
      transfer={set.permissions.transfer}
      onChanged={onTransferred}
    />
  )
}
