export function matchesReference(
  ref: string | undefined,
  resource: { id: string } | null | undefined,
) {
  return !!ref && !!resource && ref === resource.id
}

export function problemHref(problem: { problemId: string; contestId?: string; label?: string }) {
  const ref = problem.problemId
  return problem.contestId
    ? `/contests/${problem.contestId}/problems/${encodeURIComponent(problem.label || ref)}`
    : `/problems/${ref}`
}

export function submissionHref(submission: { id: string; contestId?: string }) {
  const ref = submission.id
  return submission.contestId
    ? `/contests/${submission.contestId}/submissions/${ref}`
    : `/submissions/${ref}`
}
