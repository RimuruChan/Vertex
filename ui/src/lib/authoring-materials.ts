import type {
  DomainPackageMetadata,
  DomainProgramMaterial,
  DomainTestMaterial,
  DomainGroupMaterial,
  DomainTreeEntry,
  DomainContentTree,
  DomainContentChange,
} from '@/generated/api/model'

export const materialNames: Record<string, string> = {
  generation: '生成方案',
  metadata: '基本设置',
  statement: '题面',
  program: '程序配置',
  source: '源代码',
  test: '测试点',
  group: '测试组',
  validation: '校验器自测',
  input: '输入数据',
  answer: '答案数据',
  asset: '附件',
  resource: '其他文件',
}
export const roleNames: Record<string, string> = {
  solution: '参考解',
  generator: '生成器',
  'input-validator': '输入校验器',
  'output-validator': '输出校验器',
  interactor: '交互器',
  'static-validator': '静态校验器',
}
export const isDocument = (kind: string) =>
  ['metadata', 'program', 'test', 'group', 'validation', 'generation'].includes(kind)
export const emptyTree = (): DomainContentTree => ({ entries: [] })
export const canonicalTree = (tree: DomainContentTree): DomainContentTree => ({
  entries: structuredClone(tree.entries).sort((a, b) => a.id.localeCompare(b.id)),
})
export function sameValue(a: unknown, b: unknown): boolean {
  const stable = (value: unknown): unknown =>
    Array.isArray(value)
      ? value.map(stable)
      : value && typeof value === 'object'
        ? Object.fromEntries(
            Object.entries(value)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([key, item]) => [key, stable(item)]),
          )
        : value
  return JSON.stringify(stable(a)) === JSON.stringify(stable(b))
}
export function contentChanges(
  before: DomainContentTree,
  after: DomainContentTree,
): DomainContentChange[] {
  const b = new Map(before.entries.map((entry) => [entry.id, entry])),
    a = new Map(after.entries.map((entry) => [entry.id, entry]))
  return [...new Set([...b.keys(), ...a.keys()])].sort().flatMap((id) =>
    sameValue(b.get(id), a.get(id))
      ? []
      : [
          {
            entryId: id,
            before: b.get(id),
            after: a.get(id),
            kind: !b.has(id)
              ? 'added'
              : !a.has(id)
                ? 'deleted'
                : b.get(id)?.path !== a.get(id)?.path
                  ? 'renamed'
                  : 'modified',
          },
        ],
  )
}
export function defaultMetadata(title: string, statementLanguage = 'zh'): DomainPackageMetadata {
  return {
    resourceMode: 'language-scaled',
    requirements: [],
    difficulty: 1,
    schemaVersion: 1,
    title,
    statementLanguage,
    timeLimitMs: 1000,
    memoryLimitKb: 262144,
    judgeType: 'normal',
    source: '',
    license: 'unknown',
    rightsOwner: '',
    tags: [],
    comparison: {
      kind: 'tokens',
      caseSensitive: true,
      spaceSensitive: false,
      floatingPoint: false,
      absoluteTolerance: 0,
      relativeTolerance: 0,
    },
    inputValidators: [],
    outputValidator: '',
    mainSolution: '',
    testOrder: [],
  }
}
export function defaultProgram(): DomainProgramMaterial {
  return {
    directory: '',
    schemaVersion: 1,
    name: '参考解',
    role: 'solution',
    language: 'cpp',
    protocol: 'stdio',
    files: [],
    entryPoint: '',
    arguments: [],
    expectedVerdicts: ['Accepted'],
  }
}
export function defaultTest(): DomainTestMaterial {
  return {
    schemaVersion: 1,
    name: '新测试点',
    group: '',
    isSample: false,
    isPretest: false,
    input: { kind: 'file', entry: '', generator: '', arguments: [] },
    answer: { kind: 'solution', entry: '', solution: '' },
    timeLimitMs: 0,
    memoryLimitKb: 0,
    points: 0,
    description: '',
  }
}
export function defaultGroup(): DomainGroupMaterial {
  return {
    schemaVersion: 1,
    name: '新测试组',
    aggregation: 'pass-fail',
    maxScore: 100,
    prerequisites: [],
    description: '',
  }
}
export function entryLabel(entry: DomainTreeEntry) {
  if (entry.kind === 'statement')
    return (
      (
        { zh: '中文', en: 'English', 'zh-CN': '简体中文', 'zh-TW': '繁體中文' } as Record<
          string,
          string
        >
      )[entry.attributes.language] ||
      entry.attributes.language ||
      '题面'
    )
  return entry.attributes.label || entry.path.split('/').pop() || entry.path
}

export const validationModes: [string, string][] = [
  ['invalid_input', '应拒绝的输入'],
  ['invalid_output', '应拒绝的输出'],
  ['valid_output', '应接受的输出'],
]
export const validationModeName = (mode: string) =>
  validationModes.find(([id]) => id === mode)?.[1] ?? mode
