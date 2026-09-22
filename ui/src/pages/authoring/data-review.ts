import type { DomainMaterialView, DomainTreeEntry } from '@/generated/api/model'
import { pairTestFiles, type TestPair } from './data-actions'

type SelectedFile = { file: File; index: number; part: 'input' | 'answer' }
export type ReviewedPair = TestPair & { files: SelectedFile[]; issues: string[] }

export function reviewTestFiles(files: File[]) {
  const groups = new Map<string, SelectedFile[]>()
  const rejected: { file: File; index: number; error: string }[] = []
  files.forEach((file, index) => {
    try {
      const pair = pairTestFiles([file])[0]
      const key = pair.name.toLocaleLowerCase()
      const group = groups.get(key) ?? []
      group.push({ file, index, part: pair.input ? 'input' : 'answer' })
      groups.set(key, group)
    } catch (error) {
      rejected.push({ file, index, error: (error as Error).message })
    }
  })
  const pairs: ReviewedPair[] = [...groups.values()].map((files) => {
    const pair = pairTestFiles(files.map(({ file }) => file))[0]
    const issues = [
      ...(pair.error ? [pair.error] : []),
      ...(!pair.input ? ['缺少输入文件'] : []),
      ...(!pair.answer ? ['缺少答案文件'] : []),
      ...files
        .filter(({ file }) => file.size > 64 * 1024 * 1024)
        .map(({ file }) => `${file.name} 超过 64 MiB`),
    ]
    return { ...pair, files, issues }
  })
  pairs.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
  return { pairs, rejected, readyCount: pairs.filter((pair) => !pair.issues.length).length }
}

export function missingTestMaterials(item: DomainMaterialView, entries: DomainTreeEntry[]) {
  if (!item.test) return []
  const ids = new Set(entries.map((entry) => entry.id))
  const { input, answer } = item.test
  const missing: string[] = []
  if (input.kind === 'file' && !ids.has(input.entry)) missing.push('输入文件')
  if (input.kind === 'generator' && !ids.has(input.generator)) missing.push('生成器')
  if (answer.kind === 'file' && !ids.has(answer.entry)) missing.push('答案文件')
  if (answer.kind === 'solution' && !ids.has(answer.solution)) missing.push('标准解')
  return missing
}
