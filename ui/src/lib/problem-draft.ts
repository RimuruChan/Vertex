/** Public problem numbers are only unique inside a domain. */
export function problemDraftKey(
  domain: string,
  userId: string | undefined,
  problemId: string,
  language: string,
  mock = false,
) {
  const prefix = mock ? 'vertex-mock-draft' : 'vertex-draft'
  return `${prefix}:v2:${[domain, userId ?? 'anonymous', problemId, language].map(encodeURIComponent).join(':')}`
}
