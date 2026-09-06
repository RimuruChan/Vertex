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
