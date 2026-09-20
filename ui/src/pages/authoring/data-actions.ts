import type { DomainTestMaterial, DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import type { useDomainAPI } from '@/domain/useDomainAPI'
import { defaultTest } from '@/lib/authoring-materials'

export type TestPair = { name: string; input?: File; answer?: File; error?: string }
export function pairTestFiles(files: File[]): TestPair[] {
  const pairs = new Map<string, TestPair>()
  for (const file of files) {
    const match = /^(.*)\.(in|out|ans)$/i.exec(file.name)
    if (!match) throw new Error(`${file.name} 不是 .in / .out / .ans 数据文件`)
    if (!match[1].trim()) throw new Error(`${file.name} 缺少测试点名称`)
    const key = match[1].toLocaleLowerCase(),
      pair = pairs.get(key) ?? { name: match[1] }
    const part = match[2].toLowerCase() === 'in' ? 'input' : 'answer'
    if (pair[part]) pair.error = `${pair.name} 有重复的${part === 'input' ? '输入' : '答案'}文件`
    pair[part] = file
    pairs.set(key, pair)
  }
  return [...pairs.values()].sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { numeric: true }),
  )
}
export type TestDraft = {
  entry?: DomainTreeEntry
  definition: DomainTestMaterial
  input?: File
  answer?: File
}

// Upload immutable bytes first, then atomically save descriptors and order with
// one copy token. A failed upload/CAS never exposes half of a test case.
export async function saveTestDrafts(
  api: ReturnType<typeof useDomainAPI>,
  problemId: string,
  copy: DomainWorkingCopy,
  drafts: TestDraft[],
  progress?: (done: number) => void,
) {
  if (!drafts.length || drafts.length > 100) throw new Error('每次可添加 1–100 个测试点')
  for (const draft of drafts)
    for (const file of [draft.input, draft.answer])
      if (file && file.size > 64 * 1024 * 1024) throw new Error(`${file.name} 超过 64 MiB`)
  const entries = [...copy.tree.entries],
    added: string[] = []
  const put = async (entry: Omit<DomainTreeEntry, 'blob'>, file: File) => {
    const blob = await api.postApiAuthoringProblemsIdBlobs(problemId, { file })
    const next = { ...entry, blob },
      index = entries.findIndex((e) => e.id === entry.id)
    if (index >= 0) entries[index] = next
    else entries.push(next)
    return next
  }
  for (let index = 0; index < drafts.length; index++) {
    const draft = drafts[index],
      id = draft.entry?.id ?? crypto.randomUUID(),
      definition = structuredClone(draft.definition)
    for (const [part, suffix] of [
      ['input', 'in'],
      ['answer', 'ans'],
    ] as const) {
      const file = draft[part]
      if (!file) continue
      const fileId = crypto.randomUUID()
      await put(
        {
          id: fileId,
          kind: part === 'input' ? 'input' : 'answer',
          path: `data/${id}/${fileId}.${suffix}`,
          attributes: { label: file.name },
        },
        file,
      )
      if (part === 'input')
        definition.input = { kind: 'file', entry: fileId, generator: '', arguments: [] }
      else definition.answer = { kind: 'file', entry: fileId, solution: '' }
    }
    await put(
      draft.entry ?? {
        id,
        kind: 'test',
        path: `vertex/tests/${id}.json`,
        attributes: { format: 'json', label: definition.name },
      },
      new File([JSON.stringify(definition)], 'test.json', { type: 'application/json' }),
    )
    if (!draft.entry) added.push(id)
    progress?.(index + 1)
  }
  if (added.length) {
    const metadata = await api.getApiAuthoringProblemsIdMaterialsEntryId(problemId, 'problem')
    if (!metadata.metadata) throw new Error('无法读取题目设置')
    await put(
      metadata.entry,
      new File(
        [
          JSON.stringify({
            ...metadata.metadata,
            testOrder: [...metadata.metadata.testOrder, ...added],
          }),
        ],
        'problem.json',
        { type: 'application/json' },
      ),
    )
  }
  return api.putApiAuthoringProblemsIdWorkingCopy(problemId, { etag: copy.etag, tree: { entries } })
}
export function pairDraft(pair: TestPair, isSample: boolean): TestDraft {
  if (pair.error || !pair.input || !pair.answer)
    throw new Error(pair.error ?? `${pair.name} 的输入或答案不完整`)
  return {
    definition: { ...defaultTest(), name: pair.name, isSample },
    input: pair.input,
    answer: pair.answer,
  }
}
