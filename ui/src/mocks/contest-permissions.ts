import type {
  DtoContestPermissions,
  DtoContestResponse,
  DtoUserResponse,
} from '@/generated/api/model'

export function contestPermissions(
  contest: Pick<DtoContestResponse, 'ownerId' | 'visibility' | 'admission'>,
  user: DtoUserResponse | null,
  staffRole = '',
  registered = false,
  roles: string[] = [],
): DtoContestPermissions {
  const owner = !!user && (user.role === 'admin' || user.id === contest.ownerId)
  const editor = roles.includes('editor')
  const jury = owner || staffRole === 'jury' || roles.includes('jury')
  const observer = staffRole === 'observer' || roles.includes('observer')
  const preview = owner || editor || jury || observer
  const eligible = !!user && (contest.admission === 'members' || roles.includes('participant'))
  return {
    view:
      contest.visibility !== 'private' ||
      preview ||
      roles.includes('participant') ||
      (registered && eligible),
    edit: owner || editor,
    manageAccess: owner,
    delete: owner,
    transfer: owner,
    previewProblems: preview,
    viewJury: jury || observer,
    rejudge: jury,
    reply: jury,
    eligible,
    register: eligible && !preview,
    submit: !!user && ((jury && !registered) || (eligible && registered && !preview)),
  }
}
