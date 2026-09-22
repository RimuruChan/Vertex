import type { DomainTreeEntry } from '@/generated/api/model'

// Never infer an entry from a name or test index: both can change after a check.
export function checkMaterialTarget(
  problemId: string,
  entry: Pick<DomainTreeEntry, 'id' | 'kind'>,
) {
  const base = `/authoring/${problemId}`
  const query = new URLSearchParams({ entry: entry.id })
  if (entry.kind === 'statement') return `${base}/statement?${query}`
  if (entry.kind === 'program' || entry.kind === 'source') return `${base}/programs?${query}`
  if (entry.kind === 'metadata') return `${base}/overview?${query}`
  if (['test', 'input', 'answer', 'group', 'validation', 'generation'].includes(entry.kind)) {
    if (['group', 'validation', 'generation'].includes(entry.kind)) query.set('data', entry.kind)
    if (entry.kind === 'generation') query.set('plan', entry.id)
    return `${base}/tests?${query}`
  }
}
