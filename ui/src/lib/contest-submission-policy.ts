import type { DtoContestResponse } from '@/generated/api/model'

export function canViewOtherContestSubmissions(
  contest: DtoContestResponse | null | undefined,
  now = Date.now(),
) {
  if (!contest) return false
  if (contest.permissions.viewJury) return true
  return (
    (contest.submissionVisibility === 'during' && now >= Date.parse(contest.beginAt)) ||
    (contest.submissionVisibility === 'after_end' && now > Date.parse(contest.endAt))
  )
}
