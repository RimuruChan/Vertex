import type {
  DomainBlobRef,
  DomainContentTree,
  DomainGenerationInput,
  DomainProgramSaveInput,
  DomainReviewItem,
  DomainTreeEntry,
  DomainWorkingCopy,
} from '@/generated/api/model'
import {
  contentChanges,
  defaultTest,
  entryLabel,
  isDocument,
  sameValue,
} from '@/lib/authoring-materials'
import { expandGeneration } from './generation-plan'
import { MockError } from './errors'
import { statementSections } from '@/lib/statement-structure'
import { placeMockSamples, sampleMarkdown } from './statement-samples'

type Context = {
  copy: DomainWorkingCopy
  decode: (entry: DomainTreeEntry) => string
  textBlob: (text: string) => DomainBlobRef
  save: (copy: DomainWorkingCopy, tree: DomainContentTree) => DomainWorkingCopy
}
export function mockStatementPreview(
  tree: DomainContentTree,
  entryId: string,
  content: string,
  decode: Context['decode'],
) {
  const statement =
    tree.entries.find(
      (e) => e.id === entryId && e.kind === 'statement' && e.attributes.format === 'markdown',
    ) ?? fail('题面不存在')
  const meta = tree.entries.find((e) => e.id === 'problem') ?? fail('缺少基本设置'),
    order = JSON.parse(decode(meta)).testOrder as string[]
  const samples: string[] = [],
    warnings: string[] = []
  let bytes = 0
  for (const id of order) {
    const entry = tree.entries.find((e) => e.id === id && e.kind === 'test')
    if (!entry) continue
    const test = JSON.parse(decode(entry))
    if (!test.isSample) continue
    const input = tree.entries.find((e) => e.id === test.input.entry && e.kind === 'input'),
      answer = tree.entries.find((e) => e.id === test.answer.entry && e.kind === 'answer')
    if (!input || !answer || test.input.kind !== 'file' || test.answer.kind !== 'file') {
      warnings.push(`样例「${test.name}」需要真实运行生成器和标准解。`)
      samples.push(
        sampleMarkdown(
          samples.length + 1,
          '（运行检查后可查看）',
          '（运行检查后可查看）',
          statement.attributes.language,
        ),
      )
      continue
    }
    bytes += input.blob.bytes + answer.blob.bytes
    if (input.blob.bytes > 65536 || answer.blob.bytes > 65536 || bytes > 262144) {
      warnings.push('样例过大，请下载查看。')
      continue
    }
    const a = decode(input),
      b = decode(answer)
    if (a.includes('\0') || b.includes('\0')) {
      warnings.push('二进制样例不能内联显示。')
      continue
    }
    samples.push(sampleMarkdown(samples.length + 1, a, b, statement.attributes.language))
  }
  if (!samples.length) warnings.push('尚未添加公开样例，可在测试数据中添加并标记为样例。')
  return { content: placeMockSamples(content, samples), warnings, sampleCount: samples.length }
}
const fail = (message: string): never => {
  throw new MockError(400, message)
}
export function mockGeneration(ctx: Context, input: DomainGenerationInput) {
  if (ctx.copy.mergeId) throw new MockError(409, '请先完成合并')
  let cases
  try {
    cases = expandGeneration(input.id, input.plan)
  } catch (e) {
    return fail((e as Error).message)
  }
  const entries = ctx.copy.tree.entries,
    plan = input.plan
  if (plan.generator === plan.solution) fail('生成器与标准解应是不同程序')
  for (const [id, role] of [
    [plan.generator, 'generator'],
    [plan.solution, 'solution'],
  ]) {
    const entry = entries.find((e) => e.id === id && e.kind === 'program') ?? fail('程序不存在')
    const program = JSON.parse(ctx.decode(entry))
    if (
      program.role !== role ||
      (role === 'solution' && JSON.stringify(program.expectedVerdicts) !== '["Accepted"]')
    )
      fail('请选择用途正确的生成器和标准解')
  }
  if (plan.group && !entries.some((e) => e.id === plan.group && e.kind === 'group'))
    fail('测试组不存在')
  if (entries.some((e) => e.id === input.id && e.kind !== 'generation')) fail('方案标识已被占用')
  if (input.preview) return { id: input.id, cases }
  const removed = new Set(
    entries
      .filter((e) => e.kind === 'test' && e.attributes.generationPlan === input.id)
      .map((e) => e.id),
  )
  const tree = {
    entries: entries.filter((e) => !removed.has(e.id) && e.id !== input.id && e.id !== 'problem'),
  }
  for (const item of cases) {
    if (tree.entries.some((e) => e.id === item.id)) fail('测试标识冲突')
    tree.entries.push({
      id: item.id,
      kind: 'test',
      path: `vertex/tests/${item.id}.json`,
      attributes: {
        format: 'json',
        label: item.name,
        generationPlan: input.id,
        generationRule: item.ruleId,
      },
      blob: ctx.textBlob(
        JSON.stringify({
          ...defaultTest(),
          name: item.name,
          group: plan.group,
          description: `由生成方案「${plan.name}」展开`,
          input: {
            kind: 'generator',
            entry: '',
            generator: plan.generator,
            arguments: item.arguments,
          },
          answer: { kind: 'solution', entry: '', solution: plan.solution },
        }),
      ),
    })
  }
  const metadata = entries.find((e) => e.id === 'problem') ?? fail('缺少基本设置'),
    value = JSON.parse(ctx.decode(metadata))
  const order: string[] = []
  let inserted = false
  for (const id of value.testOrder as string[]) {
    if (removed.has(id)) {
      if (!inserted) {
        order.push(...cases.map((c) => c.id))
        inserted = true
      }
    } else order.push(id)
  }
  if (!inserted) order.push(...cases.map((c) => c.id))
  value.testOrder = order
  tree.entries.push(
    { ...metadata, blob: ctx.textBlob(JSON.stringify(value)) },
    {
      id: input.id,
      kind: 'generation',
      path: `vertex/generation/${input.id}.json`,
      attributes: { format: 'json', label: plan.name },
      blob: ctx.textBlob(JSON.stringify(plan)),
    },
  )
  return { id: input.id, cases, copy: ctx.save(ctx.copy, tree) }
}

export function mockProgramSave(ctx: Context, input: DomainProgramSaveInput) {
  if (ctx.copy.mergeId) throw new MockError(409, '请先完成合并')
  const entries = structuredClone(ctx.copy.tree.entries),
    existing = entries.find((e) => e.id === input.id)
  if (existing && existing.kind !== 'program') fail('程序标识被占用')
  if ((existing?.blob.sha256 ?? '') !== input.baseHash) throw new MockError(409, '程序已改变')
  const program = structuredClone(input.program)
  if (existing) program.directory = JSON.parse(ctx.decode(existing)).directory
  const programs = entries
    .filter((e) => e.kind === 'program' && e.id !== input.id)
    .map((e) => JSON.parse(ctx.decode(e)))
  const seen = new Set<string>(),
    paths = new Set<string>()
  let isolate = !existing
  if (!input.sources.length || input.sources.length > 200) fail('代码数量无效')
  for (const source of input.sources) {
    if (
      !source.relativeName ||
      source.relativeName.startsWith('/') ||
      source.relativeName.split('/').some((p) => !p || p === '.' || p === '..') ||
      /[\\:\x00]/.test(source.relativeName) ||
      seen.has(source.id) ||
      paths.has(source.relativeName.toLowerCase())
    )
      fail('代码名称或标识无效')
    seen.add(source.id)
    paths.add(source.relativeName.toLowerCase())
    const previous = entries.find((e) => e.id === source.id)
    if (previous && previous.kind !== 'source') fail('代码类型无效')
    if ((previous?.blob.sha256 ?? '') !== source.baseHash) throw new MockError(409, '代码已变化')
    const target = [program.directory, source.relativeName].filter(Boolean).join('/')
    if (entries.some((e) => e.id !== source.id && e.path.toLowerCase() === target.toLowerCase()))
      isolate = true
    if (
      previous &&
      (previous.blob.sha256 !== source.blob.sha256 || previous.path !== target) &&
      programs.some((p) => p.files.includes(source.id))
    )
      isolate = true
  }
  if (
    !seen.has(program.entryPoint) ||
    program.files.length !== seen.size ||
    new Set(program.files).size !== seen.size ||
    program.files.some((id) => !seen.has(id))
  )
    fail('程序成员无效')
  if (isolate) program.directory = `program-${input.id}-${crypto.randomUUID()}`
  const remap: Record<string, string> = {}
  const sources = input.sources.map((source) => {
    const id = isolate && entries.some((e) => e.id === source.id) ? crypto.randomUUID() : source.id
    if (id !== source.id) remap[source.id] = id
    return {
      id,
      kind: 'source',
      path: [program.directory, source.relativeName].filter(Boolean).join('/'),
      blob: source.blob,
      attributes: {
        label: source.name || source.relativeName.split('/').pop()!,
        language: program.language,
      },
    }
  })
  program.files = program.files.map((id) => remap[id] ?? id)
  program.entryPoint = remap[program.entryPoint] ?? program.entryPoint
  const entry = {
    id: input.id,
    kind: 'program',
    path: existing?.path ?? `vertex/programs/${input.id}.json`,
    attributes: { format: 'json', label: program.name },
    blob: ctx.textBlob(JSON.stringify(program)),
  }
  const updated = new Map([...sources, entry].map((e) => [e.id, e]))
  const tree = { entries: [...entries.filter((e) => !updated.has(e.id)), ...updated.values()] }
  return { copy: ctx.save(ctx.copy, tree), program, sources, remap }
}

export function mockStructuredReview(
  before: DomainContentTree,
  after: DomainContentTree,
  decode: Context['decode'],
): DomainReviewItem[] {
  const all = new Map<string, DomainTreeEntry>(),
    labels: Record<string, string> = Object.create(null),
    owners = new Map<string, Set<string>>()
  for (const entry of [...before.entries, ...after.entries]) {
    all.set(entry.id, entry)
    labels[entry.id] =
      entry.kind === 'statement' ? '题面 · ' + entry.attributes.language : entryLabel(entry)
    if (entry.kind === 'test' && entry.attributes.generationPlan) {
      const set = owners.get(entry.id) ?? new Set<string>()
      set.add(entry.attributes.generationPlan)
      owners.set(entry.id, set)
    }
    if (!isDocument(entry.kind)) continue
    try {
      const value = JSON.parse(decode(entry))
      labels[entry.id] =
        value.name || (entry.kind === 'metadata' ? '题目与评测规则' : labels[entry.id])
      const refs =
        entry.kind === 'program'
          ? value.files
          : entry.kind === 'test'
            ? [value.input?.entry, value.answer?.entry]
            : entry.kind === 'validation'
              ? [value.input, value.answer, value.output]
              : []
      for (const ref of refs.filter(Boolean)) {
        const set = owners.get(ref) ?? new Set()
        set.add(entry.id)
        owners.set(ref, set)
      }
    } catch {
      /* raw detail still available */
    }
  }
  const result = new Map<string, DomainReviewItem>()
  const format = (value: unknown): string =>
    value === undefined || value === null
      ? ''
      : typeof value === 'string'
        ? (labels[value] ?? value)
        : Array.isArray(value)
          ? value.map(format).join('、')
          : typeof value === 'object'
            ? Object.entries(value)
                .map(([key, v]) => `${key}: ${format(v)}`)
                .join('\n')
            : String(value)
  for (const change of contentChanges(before, after)) {
    const entry = (change.after ?? change.before)!
    for (const owner of owners.get(entry.id) ?? [entry.id]) {
      const item = result.get(owner) ?? {
        id: owner,
        kind: all.get(owner)!.kind,
        label: labels[owner],
        change: 'modified',
        entryIds: [],
        fields: [],
        truncated: false,
      }
      item.entryIds.push(entry.id)
      if (owner === entry.id) item.change = change.kind
      if (isDocument(entry.kind) && owner === entry.id) {
        const a = change.before ? JSON.parse(decode(change.before)) : {},
          b = change.after ? JSON.parse(decode(change.after)) : {}
        for (const key of new Set([...Object.keys(a), ...Object.keys(b)]))
          if (!['schemaVersion', 'directory'].includes(key) && !sameValue(a[key], b[key]))
            item.fields.push({ key, before: format(a[key]), after: format(b[key]) })
      } else if (entry.kind === 'statement') {
        const before = statementSections(change.before ? decode(change.before) : ''),
          after = statementSections(change.after ? decode(change.after) : '')
        for (const key of new Set([...Object.keys(before), ...Object.keys(after)]))
          if (before[key] !== after[key])
            item.fields.push({ key, before: before[key] ?? '', after: after[key] ?? '' })
      } else
        item.fields.push({
          key: labels[entry.id],
          before: change.before ? `${change.before.blob.bytes} 字节` : '未添加',
          after: change.after ? `${change.after.blob.bytes} 字节 · 已修改` : '已移除',
        })
      result.set(owner, item)
    }
  }
  return [...result.values()]
}
