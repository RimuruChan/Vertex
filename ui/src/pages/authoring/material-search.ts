import type { DomainTreeEntry } from '@/generated/api/model'
import { entryLabel } from '@/lib/authoring-materials'

export const materialCategories = ['全部', '题面', '程序', '测试', '附件', '设置'] as const
export type MaterialCategory = (typeof materialCategories)[number]

export function materialDestination(problemId: string, entry: DomainTreeEntry) {
  const query = new URLSearchParams({ entry: entry.id })
  let section: string
  let category: MaterialCategory
  switch (entry.kind) {
    case 'statement':
      section = 'statement'
      category = '题面'
      break
    case 'program':
    case 'source':
      section = 'programs'
      category = '程序'
      break
    case 'test':
    case 'input':
    case 'answer':
    case 'group':
    case 'validation':
    case 'generation':
      section = 'tests'
      category = '测试'
      if (['group', 'validation', 'generation'].includes(entry.kind)) query.set('data', entry.kind)
      if (entry.kind === 'generation') query.set('plan', entry.id)
      break
    case 'asset':
    case 'resource':
      section = 'assets'
      category = '附件'
      break
    case 'metadata':
      section = 'overview'
      category = '设置'
      break
    default:
      return undefined
  }
  return { section, category, to: `/authoring/${problemId}/${section}?${query}` }
}

export function findMaterials(
  entries: DomainTreeEntry[],
  problemId: string,
  query: string,
  category: MaterialCategory,
  names: Record<string, string> = {},
) {
  const words = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return entries
    .flatMap((entry, order) => {
      const destination = materialDestination(problemId, entry)
      if (!destination || (category !== '全部' && destination.category !== category)) return []
      const name = entry.kind === 'metadata' ? '题目设置' : names[entry.id] || entryLabel(entry)
      const searchable =
        `${name} ${entry.path} ${destination.category} ${entry.attributes.language ?? ''}`.toLocaleLowerCase()
      if (!words.every((word) => searchable.includes(word))) return []
      const normalized = query.trim().toLocaleLowerCase()
      const score =
        normalized && name.toLocaleLowerCase() === normalized
          ? 0
          : normalized && name.toLocaleLowerCase().startsWith(normalized)
            ? 1
            : ['metadata', 'statement', 'program', 'generation', 'group'].includes(entry.kind)
              ? 2
              : 3
      return [{ entry, name, ...destination, score, order }]
    })
    .sort((a, b) => a.score - b.score || a.order - b.order)
}
