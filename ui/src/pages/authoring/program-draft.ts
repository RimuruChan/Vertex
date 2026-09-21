import type {
  DomainProgramMaterial,
  DomainTreeEntry,
  DomainWorkingCopy,
} from '@/generated/api/model'
import type { useDomainAPI } from '@/domain/useDomainAPI'
import { defaultProgram } from '@/lib/authoring-materials'
import { availableLocation } from './material-locations'

export type ProgramDraft = {
  entry: DomainTreeEntry
  program: DomainProgramMaterial
  sources: DomainTreeEntry[]
  changes: Record<string, Blob>
}
export const programLanguages: [string, string][] = [
  ['cpp', 'C++'],
  ['c', 'C'],
  ['python', 'Python'],
  ['java', 'Java'],
  ['rust', 'Rust'],
]
export const programRoles = [
  {
    id: 'solution',
    name: '参考解',
    description: '解题程序。检查时运行它，验证正确解能通过、错误解会被数据拦住。',
  },
  { id: 'generator', name: '数据生成器', description: '生成测试输入，供测试数据使用。' },
  {
    id: 'input-validator',
    name: '输入校验器',
    description: '检查测试输入是否满足题目规定的格式与范围。',
  },
  {
    id: 'output-validator',
    name: '答案校验器',
    description: '比较选手输出与标准答案，处理特殊的正确性规则。',
  },
]
export const sourceName = (language: string) =>
  ({ cpp: 'main.cpp', c: 'main.c', python: 'main.py', java: 'Main.java', rust: 'main.rs' })[
    language
  ] ?? 'main.cpp'
export function initialSource(language: string) {
  return (
    {
      cpp: '#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n\n    return 0;\n}\n',
      c: '#include <stdio.h>\n\nint main(void) {\n    return 0;\n}\n',
      python: 'import sys\n\ndef main():\n    pass\n\nif __name__ == "__main__":\n    main()\n',
      java: 'public class Main {\n    public static void main(String[] args) {\n    }\n}\n',
      rust: 'fn main() {\n}\n',
    }[language] ?? ''
  )
}
export function newProgram(name: string, role: string, language: string): ProgramDraft {
  const id = crypto.randomUUID(),
    sourceId = crypto.randomUUID(),
    filename = sourceName(language)
  const directory = `sources/${id}`
  return {
    entry: {
      id,
      kind: 'program',
      path: `vertex/programs/${id}.json`,
      attributes: { label: name, format: 'json' },
      blob: { sha256: '', bytes: 0 },
    },
    program: {
      ...defaultProgram(),
      name,
      role,
      language,
      directory,
      files: [sourceId],
      entryPoint: sourceId,
      expectedVerdicts: role === 'solution' ? ['Accepted'] : [],
      protocol: 'stdio',
    },
    sources: [
      {
        id: sourceId,
        kind: 'source',
        path: `${directory}/${filename}`,
        attributes: { label: filename, language },
        blob: { sha256: '', bytes: 0 },
      },
    ],
    changes: {
      [sourceId]: new Blob(
        [role === 'solution' || role === 'generator' ? initialSource(language) : ''],
        { type: 'text/plain' },
      ),
    },
  }
}

/** One logical program save is one copy CAS, even when it changes multiple sources. */
export async function saveProgramDraft(
  api: ReturnType<typeof useDomainAPI>,
  problemId: string,
  copy: DomainWorkingCopy,
  draft: ProgramDraft,
  otherPrograms: DomainProgramMaterial[],
) {
  for (const original of [draft.entry, ...draft.sources]) {
    if (
      original.blob.sha256 &&
      copy.tree.entries.find((e) => e.id === original.id)?.blob.sha256 !== original.blob.sha256
    )
      throw new Error('此程序已在另一页面修改。本地代码已保留，请核对后重新读取。')
  }
  const program = structuredClone(draft.program),
    sources = structuredClone(draft.sources)
  const entries = [...copy.tree.entries]
  const shared = sources.some((source) =>
    otherPrograms.some((other) => other.files.includes(source.id)),
  )
  const occupied = sources.some(
    (source) =>
      !copy.tree.entries.some((e) => e.id === source.id) &&
      copy.tree.entries.some((e) => {
        const a = source.path.toLowerCase(),
          b = e.path.toLowerCase()
        return a === b || a.startsWith(b + '/') || b.startsWith(a + '/')
      }),
  )
  const remap = new Map<string, string>()
  // A shared imported source must not silently edit another program. Clone the
  // whole layout, retaining relative includes, when this program's code changes.
  if (occupied || (shared && Object.keys(draft.changes).length)) {
    const root = program.directory ? program.directory + '/' : ''
    const nextRoot = availableLocation(
      `sources/${draft.entry.id}-${crypto.randomUUID()}`,
      entries,
      crypto.randomUUID(),
    )
    for (const source of sources) {
      const previous = source.id
      source.id = crypto.randomUUID()
      remap.set(previous, source.id)
      source.path = `${nextRoot}/${source.path.startsWith(root) ? source.path.slice(root.length) : source.path.split('/').pop()}`
    }
    program.files = program.files.map((id) => remap.get(id) ?? id)
    program.entryPoint = remap.get(program.entryPoint) ?? program.entryPoint
    program.directory = nextRoot
  }
  for (const source of sources) {
    const originalId = [...remap].find(([, id]) => id === source.id)?.[0] ?? source.id
    const changed = draft.changes[originalId]
    if (changed)
      source.blob = await api.postApiAuthoringProblemsIdBlobs(problemId, { file: changed })
    if (!source.blob.sha256) throw new Error('代码未完成上传，请重试。')
    const index = entries.findIndex((e) => e.id === source.id)
    if (index < 0) entries.push(source)
    else entries[index] = source
  }
  const entry = {
    ...draft.entry,
    path: availableLocation(
      draft.entry.path,
      entries.filter((e) => e.id !== draft.entry.id),
      draft.entry.id,
    ),
    attributes: { ...draft.entry.attributes, label: program.name },
    blob: await api.postApiAuthoringProblemsIdBlobs(problemId, {
      file: new Blob([JSON.stringify(program)], { type: 'application/json' }),
    }),
  }
  const index = entries.findIndex((e) => e.id === entry.id)
  if (index < 0) entries.push(entry)
  else entries[index] = entry
  return {
    copy: await api.putApiAuthoringProblemsIdWorkingCopy(problemId, {
      etag: copy.etag,
      tree: { entries },
    }),
    program,
    sources,
    remap,
  }
}
