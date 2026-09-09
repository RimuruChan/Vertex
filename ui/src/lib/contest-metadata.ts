export function showContestProblemMetadata(
  contest: { endAt: string; showProblemMetadata?: boolean } | null | undefined,
  now = Date.now(),
) {
  return !!contest && (contest.showProblemMetadata === true || now >= Date.parse(contest.endAt))
}
