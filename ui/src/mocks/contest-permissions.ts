import type {
  DtoContestPermissions,
  DtoContestResponse,
  DtoUserResponse,
} from '@/generated/api/model'
import { mockActive, mockCan, mockManager, type MockScope } from './domain-policy'

export function contestPermissions(
  contest: Pick<DtoContestResponse, 'ownerId' | 'visibility' | 'admission'>,
  user: DtoUserResponse | null,
  staffRole = '',
  registered = false,
  roles: string[] = [],
  scope?: MockScope,
): DtoContestPermissions {
  const owner =
    mockManager(scope, user) || (mockActive(scope, user) && user?.id === contest.ownerId)
  const writable = !scope?.archived
  if (!mockActive(scope, user)) {
    roles = []
    staffRole = ''
  }
  const editor = roles.includes('editor')
  const jury = owner || staffRole === 'jury' || roles.includes('jury')
  const observer = staffRole === 'observer' || roles.includes('observer')
  const preview = owner || editor || jury || observer
  const eligible =
    mockCan(scope, user, 'submission.create') &&
    (contest.admission === 'members' || roles.includes('participant'))
  return {
    view:
      contest.visibility !== 'private' ||
      preview ||
      roles.includes('participant') ||
      (registered && eligible),
    edit: writable && (owner || editor),
    manageAccess: writable && owner,
    delete: writable && owner,
    transfer: writable && owner,
    previewProblems: preview,
    viewJury: jury || observer,
    rejudge: writable && jury,
    reply: writable && jury,
    eligible,
    register: writable && eligible && !preview,
    submit:
      writable &&
      mockCan(scope, user, 'submission.create') &&
      ((jury && !registered) || (eligible && registered && !preview)),
  }
}
