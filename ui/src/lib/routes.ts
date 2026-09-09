export function matchesReference(
  ref: string | undefined,
  resource: { id: string; publicId?: string } | null | undefined,
) {
  return !!ref && !!resource && (ref === resource.id || ref === resource.publicId)
}

export function problemHref(problem: {
  problemId: string
  problemPublicId?: string
  contestId?: string
  contestPublicId?: string
  label?: string
}) {
  const ref = problem.problemPublicId || problem.problemId
  return problem.contestId
    ? `/contests/${problem.contestPublicId || problem.contestId}/problems/${encodeURIComponent(problem.label || ref)}`
    : `/problems/${ref}`
}

export function submissionHref(submission: {
  id: string
  publicId?: string
  contestId?: string
  contestPublicId?: string
}) {
  const ref = submission.publicId || submission.id
  return submission.contestId
    ? `/contests/${submission.contestPublicId || submission.contestId}/submissions/${ref}`
    : `/submissions/${ref}`
}
