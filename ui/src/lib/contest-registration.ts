import type { DtoContestResponse } from '@/generated/api/model'

type RegistrationSettings = Pick<
  DtoContestResponse,
  'allowSelfRegistration' | 'allowLateRegistration' | 'beginAt' | 'endAt'
>

// This displayed window is not authorization: the server also checks current
// membership, contest roles and passwords under the registration lock.
export function registrationWindow(contest: RegistrationSettings, now = Date.now()) {
  const begin = Date.parse(contest.beginAt),
    end = Date.parse(contest.endAt)
  if (!Number.isFinite(begin) || !Number.isFinite(end) || now >= end) return 'ended'
  if (!contest.allowSelfRegistration) return 'disabled'
  if (!contest.allowLateRegistration && now >= begin) return 'closed'
  return 'open'
}
