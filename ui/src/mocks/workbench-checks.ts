import type {
  DomainCheckRun,
  DomainCommitRelease,
  DomainContentTree,
  DomainMaterialInspection,
  DomainPackageMetadata,
  DomainProgramMaterial,
  DomainTestMaterial,
  DomainValidationMaterial,
  DomainTreeEntry,
  DtoProblemPermissions,
  DtoProblemResponse,
} from '@/generated/api/model'
import type { MockWorkbench } from './workbench'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'
import { sameValue } from '@/lib/authoring-materials'

export type SavedMockCheck = {
  actor: string
  tree: DomainContentTree
  run: DomainCheckRun
  steps: number
}
const fail = (status: number, message: string): never => {
  throw new MockError(status, message)
}
// Deterministic only for demo identity comparisons, never a cryptographic hash.
export function mockContentHash(value: unknown): string {
  const text = JSON.stringify(value)
  let hash = 2166136261
  for (let index = 0; index < text.length; index++)
    hash = Math.imul(hash ^ text.charCodeAt(index), 16777619)
  return (hash >>> 0).toString(16).padStart(8, '0').repeat(8)
}
function inspect(
  tree: DomainContentTree,
  decode: (entry: DomainTreeEntry) => string,
  etag?: string,
  revision?: number,
) {
  const meta = tree.entries.find((item) => item.id === 'problem'),
    issues: DomainMaterialInspection['issues'] = []
  const metadata = meta ? (JSON.parse(decode(meta)) as DomainPackageMetadata) : undefined
  const programs = tree.entries
    .filter((item) => item.kind === 'program')
    .map((entry) => ({ entry, definition: JSON.parse(decode(entry)) as DomainProgramMaterial }))
  const validation = tree.entries
    .filter((item) => item.kind === 'validation')
    .map((entry) => ({ entry, definition: JSON.parse(decode(entry)) as DomainValidationMaterial }))
  const tests = (metadata?.testOrder ?? []).map((id) => {
    const entry = tree.entries.find((item) => item.id === id)
    return entry
      ? { entry, definition: JSON.parse(decode(entry)) as DomainTestMaterial }
      : undefined
  })
  const error = (code: string, entryId: string, message: string) =>
    issues.push({ code, entryId, message, severity: 'error' })
  if (!metadata) error('metadata.required', 'problem', '缺少基本设置')
  if (!tests.length) error('test.required', 'problem', '至少添加一个测试点')
  if (
    !programs.some(
      (item) => item.entry.id === metadata?.mainSolution && item.definition.role === 'solution',
    )
  )
    error('solution.required', 'problem', '指定主参考解')
  if (metadata?.judgeType !== 'normal')
    error('judge.unsupported', 'problem', '当前演示检查仅支持传统题')
  for (const requirement of metadata?.requirements ?? [])
    if (requirement.stage === 'build') error(requirement.code, 'problem', requirement.message)
  for (const { entry, definition } of programs) {
    if (
      !definition.files.length ||
      definition.files.some(
        (id) => !tree.entries.some((item) => item.id === id && item.kind === 'source'),
      )
    )
      error('program.files', entry.id, '程序缺少源代码文件')
  }
  for (const test of tests) {
    if (!test) {
      error('test.missing', 'problem', '测试顺序引用不存在的测试点')
      continue
    }
    if (
      test.definition.input.kind === 'file' &&
      !tree.entries.some((item) => item.id === test.definition.input.entry && item.kind === 'input')
    )
      error('test.input', test.entry.id, '测试输入尚未配置')
    if (
      test.definition.answer.kind === 'file' &&
      !tree.entries.some(
        (item) => item.id === test.definition.answer.entry && item.kind === 'answer',
      )
    )
      error('test.answer', test.entry.id, '测试答案尚未配置')
  }
  for (const { entry, definition } of validation) {
    if (!['invalid_input', 'invalid_output', 'valid_output'].includes(definition.mode))
      error('validation.mode', entry.id, '未知自测预期')
    const references: [string | undefined, string][] = [[definition.input, 'input']]
    if (definition.mode === 'invalid_input') {
      if (!metadata?.inputValidators.length)
        error('validation.validator_required', entry.id, '请指定输入校验器')
      if (definition.answer || definition.output)
        error('validation.files', entry.id, '无效输入自测不应包含答案或输出')
    } else references.push([definition.answer, 'answer'], [definition.output, 'answer'])
    for (const [id, kind] of references)
      if (!tree.entries.some((item) => item.id === id && item.kind === kind))
        error('validation.reference', entry.id, '自测文件尚未配置')
  }
  const data = tree.entries.filter(
    (entry) => !['statement', 'asset', 'resource', 'metadata'].includes(entry.kind),
  )
  const policy = metadata && {
    timeLimitMs: metadata.timeLimitMs,
    memoryLimitKb: metadata.memoryLimitKb,
    resourceMode: metadata.resourceMode,
    judgeType: metadata.judgeType,
    comparison: metadata.comparison,
    inputValidators: metadata.inputValidators,
    outputValidator: metadata.outputValidator,
    mainSolution: metadata.mainSolution,
    testOrder: metadata.testOrder,
  }
  const report: DomainMaterialInspection = {
    canBuild: !issues.length,
    treeHash: mockContentHash(tree),
    dataHash: mockContentHash([data, policy]),
    policyVersion: 'vertex-authoring-4',
    issues,
    publicationIssues:
      tests.some((test) => test && (test.definition.points !== 0 || test.definition.isPretest)) ||
      tree.entries.some((entry) => entry.kind === 'group')
        ? [
            {
              severity: 'error',
              code: 'publication.unsupported',
              message: '分组计分、逐点分值或预测试阶段尚未接入提交评测，暂不能发布',
            },
          ]
        : [],
    validationCount: validation.length,
    programCount: programs.length,
    testCount: tests.length,
    sampleCount: tests.filter((test) => test?.definition.isSample).length,
    etag,
    revision,
  }
  return {
    report,
    metadata,
    programs,
    validation,
    tests: tests.filter((item) => item !== undefined),
  }
}

export function mockCheckRequest(context: {
  store: MockWorkbench
  state: MockState
  problem: DtoProblemResponse
  permissions: DtoProblemPermissions
  request: MockRequest
  now: number
  actor: string
  decode: (entry: DomainTreeEntry) => string
}): unknown {
  const { store, state, problem, permissions, request, now, actor, decode } = context
  const { method, body = {}, params = {} } = request,
    [, , , , section, child, action] = request.path.split('/').filter(Boolean)
  const checks = (store.checks ??= []),
    releases = (store.releases ??= []),
    time = new Date(now).toISOString()
  const readable = (item: SavedMockCheck) =>
    item.actor === actor ||
    Boolean(item.run.revision) ||
    store.commits.some((commit) => sameValue(commit.tree, item.tree))
  const wire = (item: SavedMockCheck) => ({
    ...item.run,
    matchingRevision: store.commits.find((commit) => sameValue(commit.tree, item.tree))?.commit
      .revision,
  })
  const select = (revision: unknown, etag: unknown) => {
    if (Number(revision) > 0)
      return (
        store.commits.find((item) => item.commit.revision === Number(revision))?.tree ??
        fail(404, '提交不存在')
      )
    const copy = store.copies[actor] ?? fail(404, '尚未创建工作副本')
    if (etag && copy.etag !== etag) fail(409, '工作副本已变化')
    if (copy.mergeId) fail(409, '请先解决冲突')
    return copy.tree
  }
  if (section === 'inspection')
    return inspect(
      select(params.revision, params.etag),
      decode,
      params.revision ? undefined : store.copies[actor]?.etag,
      Number(params.revision) || undefined,
    ).report
  if (section === 'checks') {
    if (method === 'POST' && !child) {
      const tree = select(body.revision, body.etag),
        { report } = inspect(tree, decode)
      if (!report.canBuild) fail(400, '请先处理材料引用问题')
      const previous = checks.find(
        (item) =>
          item.actor === actor &&
          item.run.treeHash === report.treeHash &&
          ['queued', 'running'].includes(item.run.state),
      )
      if (previous) return wire(previous)
      const run: DomainCheckRun = {
        id: crypto.randomUUID(),
        treeHash: report.treeHash,
        dataHash: report.dataHash!,
        policyVersion: report.policyVersion,
        revision: Number(body.revision) || undefined,
        state: 'queued',
        stage: 'queued',
        progressDone: 0,
        progressTotal: report.testCount,
        packageCases: 0,
        createdAt: time,
        log: '本地演示检查：未执行任何程序，结果用于预览交互。',
      }
      const saved = { actor, tree: structuredClone(tree), run, steps: 0 }
      checks.unshift(saved)
      return wire(saved)
    }
    if (child) {
      const item =
        checks.find((item) => item.run.id === child && readable(item)) ?? fail(404, '检查不存在')
      if (
        action === 'cancel' &&
        method === 'POST' &&
        ['queued', 'running'].includes(item.run.state)
      ) {
        item.run.state = 'cancelled'
        item.run.finishedAt = time
        item.run.stage = 'done'
      }
      return wire(item)
    }
    for (const item of checks.filter(readable)) {
      if (!['queued', 'running'].includes(item.run.state)) continue
      item.steps++
      if (item.steps < 3) {
        item.run.state = 'running'
        item.run.stage = 'verifying'
        item.run.startedAt ??= time
        continue
      }
      const { tests, programs, validation } = inspect(item.tree, decode)
      item.run.validation = validation.map(({ entry, definition }) => ({
        id: entry.id,
        name: definition.name,
        mode: definition.mode,
        actual: definition.mode === 'valid_output' ? 'accepted' : 'rejected',
        status: 'ok',
        message: '演示结果，未实际运行校验器',
      }))
      const file = (id: string) => item.tree.entries.find((entry) => entry.id === id)
      item.run.tests = tests.map((test, index) => ({
        index: index + 1,
        isSample: test.definition.isSample,
        status: 'ok',
        inputBytes: file(test.definition.input.entry)?.blob.bytes ?? 0,
        answerBytes: file(test.definition.answer.entry)?.blob.bytes ?? 0,
        inputHead: file(test.definition.input.entry)
          ? decode(file(test.definition.input.entry)!).slice(0, 1024)
          : undefined,
        answerHead: file(test.definition.answer.entry)
          ? decode(file(test.definition.answer.entry)!).slice(0, 1024)
          : undefined,
        timeMs: 2,
        memoryKb: 1024,
        source: test.definition.input.kind,
        points: test.definition.points,
      }))
      item.run.solutions = programs
        .filter((item) => item.definition.role === 'solution')
        .map(({ definition }) => ({
          name: definition.name,
          language: definition.language,
          expectedVerdict: definition.expectedVerdicts.join(' / '),
          actualVerdict: definition.expectedVerdicts[0] || 'Accepted',
          matched: true,
          maxTimeMs: 2,
          maxMemoryKb: 1024,
          cases: tests.map((_, index) => ({
            index: index + 1,
            verdict: definition.expectedVerdicts[0] || 'Accepted',
            timeMs: 2,
            memoryKb: 1024,
          })),
        }))
      Object.assign(item.run, {
        state: 'succeeded',
        stage: 'done',
        progressDone: tests.length,
        packageCases: tests.length,
        finishedAt: time,
        toolchainKey: 'demo-toolchain',
      })
    }
    const items = checks
      .filter(readable)
      .slice(0, Number(params.limit) || 30)
      .map(wire)
    return { items, total: items.length }
  }
  if (section === 'releases') {
    if (method === 'GET') return { items: releases, total: releases.length }
    if (!permissions.publish) fail(403, '没有发布权限')
    const commit =
      store.commits.find((item) => item.commit.revision === Number(body.revision)) ??
      fail(404, '提交不存在')
    const check =
      checks.find((item) => item.run.id === body.checkId && readable(item)) ??
      fail(404, '检查不存在')
    const { report, metadata, tests } = inspect(commit.tree, decode)
    const language = String(body.language || metadata?.statementLanguage)
    const latest = releases[0]
    if (
      latest?.revision === Number(body.revision) &&
      latest.checkId === body.checkId &&
      latest.language === language
    )
      return latest
    if (Number(body.expectedVersion) !== problem.publishedVersion)
      fail(409, '公开版本已变化，请刷新后核对')
    if (
      !report.canBuild ||
      report.publicationIssues.length > 0 ||
      metadata?.requirements?.length ||
      check.run.state !== 'succeeded' ||
      check.run.dataHash !== report.dataHash ||
      check.run.policyVersion !== report.policyVersion
    )
      fail(400, '缺少匹配的成功检查')
    const statement = commit.tree.entries.find(
      (entry) => entry.kind === 'statement' && entry.attributes.language === language,
    )
    if (!statement || statement.attributes.format !== 'markdown')
      return fail(400, '题面语言或格式不能发布')
    if (/\{\{(?:nextsample|remainingsamples)\}\}/.test(decode(statement)))
      fail(400, '演示环境尚未渲染样例插入指令，请使用真实后端检查排版')
    const value: DomainCommitRelease = {
      version: problem.publishedVersion + 1,
      revision: commit.commit.revision,
      treeHash: report.treeHash,
      checkId: check.run.id,
      toolchainKey: check.run.toolchainKey!,
      language,
      createdAt: time,
    }
    const fence = (text: string) => {
      const delimiter = '`'.repeat(
        Math.max(3, ...(text.match(/`+/g) ?? []).map((run) => run.length + 1)),
      )
      return `${delimiter}\n${text.trimEnd()}\n${delimiter}`
    }
    const samples = tests
      .filter((test) => test.definition.isSample)
      .map((test, index) => {
        const input = commit.tree.entries.find((entry) => entry.id === test.definition.input.entry)
        const answer = commit.tree.entries.find(
          (entry) => entry.id === test.definition.answer.entry,
        )
        if (!input || !answer)
          return fail(400, '演示环境不能执行程序生成样例，请提供输入和答案文件')
        const zh = language === 'zh'
        return `### ${zh ? '样例' : 'Example'} ${index + 1}\n\n**${zh ? '输入' : 'Input'}**\n\n${fence(decode(input))}\n\n**${zh ? '输出' : 'Output'}**\n\n${fence(decode(answer))}`
      })
    Object.assign(problem, {
      publishedVersion: value.version,
      title: metadata!.title,
      statementMd: decode(statement) + (samples.length ? '\n\n' + samples.join('\n\n') : ''),
      timeLimitMs: metadata!.timeLimitMs,
      memoryLimitKb: metadata!.memoryLimitKb,
      tags: [...metadata!.tags],
      difficulty: metadata!.difficulty,
      source: metadata!.source,
      updatedAt: time,
    })
    releases.unshift(value)
    state.problemReleases[problem.id] = [
      {
        release: { version: value.version },
        problem: structuredClone(problem),
      },
      ...(state.problemReleases[problem.id] ?? []),
    ]
    return value
  }
  return fail(501, '未实现的演示操作')
}
