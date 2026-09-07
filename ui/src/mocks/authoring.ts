import type {
  DtoFileResponse,
  DtoProblemResponse,
  DtoStatementResponse,
  DtoTestResponse,
  DtoWorkspaceResponse,
} from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'
import { allocateReference } from './references'
import { officialDomainID, problemPermissions } from './problem-permissions'
import { mockCan } from './domain-policy'

const sample = (markdown: string, label: string) =>
  markdown.split(`## ${label}\n\n`)[1]?.split('\n\n## ')[0] ?? ''
const codeBlock = (value: string) => value.replace(/^```\w*\n/, '').replace(/\n```$/, '')

function initialWorkspace(problem: DtoProblemResponse): DtoWorkspaceResponse {
  return {
    meta: {
      problemId: problem.id,
      problemPublicId: problem.publicId,
      title: problem.title,
      visibility: problem.visibility,
      timeLimitMs: problem.timeLimitMs,
      memoryLimitKb: problem.memoryLimitKb,
      judgeType: problem.judgeType,
      statementLanguage: 'zh',
      packageRevision: 1,
      dataRevision: 1,
      publishedVersion: problem.publishedVersion,
      publishedRevision: problem.publishedVersion ? 1 : -1,
      publishedArtifactVersion: problem.publishedVersion ? 1 : 0,
      unpublishedChanges: !problem.publishedVersion,
      canEdit: false,
      canPublish: false,
      builtRevision: problem.publishedVersion ? 1 : 0,
      stale: !problem.publishedVersion,
      testdataVersion: problem.publishedVersion ? 1 : 0,
      testdataCases: problem.publishedVersion ? 1 : 0,
      testdataChecker: 'diff',
      testdataSha256: problem.publishedVersion ? 'mock-initial' : '',
    },
    statements: [
      {
        language: 'zh',
        name: problem.title,
        legend: problem.statementMd.split('\n\n## ')[0],
        inputFormat: sample(problem.statementMd, '输入格式'),
        outputFormat: sample(problem.statementMd, '输出格式'),
        notes: '',
        scoring: '',
        tutorial: '',
        updatedAt: problem.updatedAt,
      },
    ],
    files: [
      {
        id: 1,
        kind: 'solution',
        name: 'main.cpp',
        language: 'cpp',
        sourceCode:
          '// 本地演示标程，可编辑；mock 构建不会执行代码。\n#include <iostream>\nint main() {\n    std::cout << "1 2\\n";\n}\n',
        isActive: true,
        expectedVerdict: 'Accepted',
        updatedAt: problem.updatedAt,
      },
    ],
    tests: [
      {
        id: 1,
        index: 1,
        description: '题面样例',
        inputData: codeBlock(sample(problem.statementMd, '样例输入')),
        source: 'manual',
        generateCmd: '',
        group: 'samples',
        isSample: true,
        points: 100,
      },
    ],
    issues: [],
  }
}

function renderStatement(
  statement: DtoStatementResponse,
  samples: { input: string; answer: string }[],
) {
  const zh = statement.language === 'zh'
  const section = (title: string, value: string) =>
    value.trim() ? `## ${title}\n\n${value.trimEnd()}` : ''
  const fence = (value: string) => {
    const length = Math.max(3, ...(value.match(/`+/g) ?? []).map((run) => run.length + 1))
    const delimiter = '`'.repeat(length)
    return `${delimiter}\n${value.trimEnd()}\n${delimiter}`
  }
  return (
    [
      section(zh ? '题目描述' : 'Statement', statement.legend),
      section(zh ? '输入格式' : 'Input', statement.inputFormat),
      section(zh ? '输出格式' : 'Output', statement.outputFormat),
      samples.length
        ? `## ${zh ? '样例' : 'Examples'}\n\n` +
          samples
            .map(
              (sample, index) =>
                (samples.length > 1 ? `### ${zh ? '样例' : 'Example'} ${index + 1}\n\n` : '') +
                `**${zh ? '输入' : 'Input'}**\n\n${fence(sample.input)}\n\n**${zh ? '输出' : 'Output'}**\n\n${fence(sample.answer)}`,
            )
            .join('\n\n')
        : '',
      section(zh ? '计分方式' : 'Scoring', statement.scoring),
      section(zh ? '说明与提示' : 'Notes', statement.notes),
    ]
      .filter(Boolean)
      .join('\n\n') + '\n'
  )
}

export function authoringRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
): unknown {
  const [, , resource, id, section, itemId, action] = path.split('/').filter(Boolean)
  const get = method === 'GET',
    post = method === 'POST',
    del = method === 'DELETE'
  const iso = new Date(now).toISOString()
  const text = (key: string) => (typeof body[key] === 'string' ? (body[key] as string) : '')
  const required = (key: string) => {
    const value = text(key).trim()
    if (!value) throw new MockError(400, '请填写必填内容。')
    return value
  }
  if (resource === 'package-templates' && get)
    return {
      items: [
        {
          kind: 'solution',
          name: 'main.cpp',
          title: 'C++ 标程骨架',
          language: 'cpp',
          description: '在这里实现解法；mock 模式不执行程序。',
          sourceCode: '#include <iostream>\nint main() {\n    return 0;\n}\n',
        },
      ],
      total: 1,
    }
  if (resource !== 'problems')
    throw new MockError(501, '此管理接口尚未提供 mock，未向真实后端发送请求。')
  if (!id && get) {
    const items = state.problems
      .map((p) => ({
        ...p,
        ...state.problemDrafts[p.id],
        visibility: p.visibility,
        ownerId: p.ownerId,
      }))
      .filter(
        (p) =>
          problemPermissions(p, state.user, state.scope).readPackage &&
          p.title.includes(String(params.keyword ?? '')) &&
          (!params.visibility || p.visibility === params.visibility),
      )
    const size = Number(params.size) || 20,
      page = Number(params.page) || 1
    return {
      items: items
        .slice((page - 1) * size, page * size)
        .map((p) => ({ ...p, permissions: problemPermissions(p, state.user, state.scope) })),
      total: items.length,
    }
  }
  if (!id && post) {
    if (!state.user) throw new MockError(401, '请先登录。')
    if (!mockCan(state.scope, state.user, 'problem.create'))
      throw new MockError(403, '当前域没有创建题目权限。')
    const problem: DtoProblemResponse = {
      publishedVersion: 0,
      ownerId: state.user.id,
      domainId: state.scope?.id ?? officialDomainID,
      permissions: problemPermissions(
        { ownerId: state.user.id, visibility: text('visibility') || 'draft' },
        state.user,
      ),
      publicId: allocateReference(state, 'problems'),
      id: crypto.randomUUID(),
      title: required('title'),
      statementMd: text('statementMd'),
      difficulty: Number(body.difficulty) || 1,
      source: text('source'),
      timeLimitMs: Number(body.timeLimitMs) || 1000,
      memoryLimitKb: Number(body.memoryLimitKb) || 262144,
      visibility: text('visibility') || 'draft',
      judgeType: 'normal',
      tags: Array.isArray(body.tags) ? (body.tags as string[]) : [],
      authorId: state.user?.id,
      userStatus: 'none',
      acceptedCount: 0,
      submissionCount: 0,
      solvedUserCount: 0,
      createdAt: iso,
      updatedAt: iso,
    }
    state.problems.unshift(problem)
    return problem
  }
  const problem = state.problems.find((p) => p.id === id)
  if (!problem) throw new MockError(404, '演示题目不存在。')
  problem.permissions = problemPermissions(problem, state.user, state.scope)
  if (!problem.permissions.readPackage)
    throw new MockError(get ? 404 : 403, '没有此题目的协作权限。')
  const workspace = (state.workspaces[id] ??= initialWorkspace(problem))
  const draft = (state.problemDrafts[id] ??= structuredClone(problem))
  state.problemCandidateSamples[id] ??=
    problem.publishedVersion && sample(problem.statementMd, '样例输入')
      ? [
          {
            input: codeBlock(sample(problem.statementMd, '样例输入')),
            answer: codeBlock(sample(problem.statementMd, '样例输出')),
          },
        ]
      : []
  workspace.meta.dataRevision ??= workspace.meta.packageRevision
  workspace.meta.publishedVersion ??= problem.publishedVersion
  workspace.meta.publishedRevision ??= problem.publishedVersion
    ? workspace.meta.packageRevision
    : -1
  workspace.meta.publishedArtifactVersion ??= problem.publishedVersion
    ? workspace.meta.testdataVersion
    : 0
  workspace.meta.canEdit = problem.permissions.edit
  workspace.meta.canPublish = problem.permissions.publish
  const refreshFlags = () => {
    workspace.meta.stale =
      workspace.meta.testdataCases === 0 ||
      workspace.meta.dataRevision !== workspace.meta.builtRevision
    workspace.meta.unpublishedChanges =
      workspace.meta.packageRevision !== workspace.meta.publishedRevision ||
      workspace.meta.testdataVersion !== workspace.meta.publishedArtifactVersion
  }
  const touch = (data = true) => {
    workspace.meta.packageRevision++
    if (data) workspace.meta.dataRevision++
    draft.updatedAt = iso
    refreshFlags()
  }
  const draftView = () => ({
    ...draft,
    visibility: problem.visibility,
    ownerId: problem.ownerId,
    permissions: problem.permissions,
    publishedVersion: problem.publishedVersion,
  })
  if (!get && !(post && action === 'preview') && !problem.permissions.edit)
    throw new MockError(403, '没有编辑权限。')
  refreshFlags()
  if (section === 'testdata' && post) {
    if (!(body.file instanceof Blob) || body.file.size === 0)
      throw new MockError(400, '请选择非空 ZIP 文件。')
    touch(true)
    state.problemCandidateSamples[id] = []
    Object.assign(workspace.meta, {
      builtRevision: workspace.meta.dataRevision,
      testdataCases: 1,
      testdataVersion: workspace.meta.testdataVersion + 1,
      testdataChecker: 'diff',
      testdataSha256: 'mock-upload-' + workspace.meta.packageRevision,
    })
    refreshFlags()
    return { caseCount: 1, sha256: workspace.meta.testdataSha256 }
  }
  if (!section) {
    if (get) return draftView()
    if (del) {
      if (!problem.permissions.delete) throw new MockError(403, '没有删除权限。')
      if (
        state.submissions.some((s) => s.problemId === id) ||
        Object.values(state.contestProblemIds).some((items) => items.includes(id))
      )
        throw new MockError(409, '已有比赛或提交引用，请隐藏题目。')
      state.problems = state.problems.filter((p) => p.id !== id)
      delete state.workspaces[id]
      delete state.problemDrafts[id]
      delete state.problemCandidateSamples[id]
      delete state.problemReleases[id]
      return { status: 'ok' }
    }
    if (method === 'PUT') {
      if ((text('visibility') || 'draft') !== problem.visibility && !problem.permissions.publish)
        throw new MockError(403, '没有更改可见性的权限。')
      const dataChanged =
        (Number(body.timeLimitMs) || 1000) !== draft.timeLimitMs ||
        (Number(body.memoryLimitKb) || 262144) !== draft.memoryLimitKb
      Object.assign(draft, {
        title: required('title'),
        statementMd: text('statementMd'),
        difficulty: Number(body.difficulty) || 1,
        source: text('source'),
        timeLimitMs: Number(body.timeLimitMs) || 1000,
        memoryLimitKb: Number(body.memoryLimitKb) || 262144,
        visibility: text('visibility') || 'draft',
        tags: Array.isArray(body.tags) ? body.tags : [],
        updatedAt: iso,
      })
      problem.visibility = draft.visibility
      const primary = workspace.statements.find(
        (statement) => statement.language === workspace.meta.statementLanguage,
      )
      if (primary) {
        primary.name = draft.title
        primary.updatedAt = iso
      }
      Object.assign(workspace.meta, {
        title: draft.title,
        timeLimitMs: draft.timeLimitMs,
        memoryLimitKb: draft.memoryLimitKb,
        visibility: problem.visibility,
      })
      touch(dataChanged)
      return draftView()
    }
  }
  const build = workspace.latestBuild
  if (build && ['queued', 'running'].includes(build.state)) {
    const elapsed = now - Date.parse(build.createdAt)
    build.state = elapsed < 600 ? 'queued' : elapsed < 4500 ? 'running' : 'succeeded'
    build.stage =
      elapsed < 600 ? 'queued' : elapsed < 2500 ? 'compile' : elapsed < 4500 ? 'generate' : 'done'
    build.progressDone = Math.min(build.progressTotal, Math.floor(elapsed / 900))
    build.log = '本地模拟构建：未编译或执行程序，未上传或发布真实测试数据。'
    if (elapsed >= 4500) {
      build.progressDone = build.progressTotal
      build.finishedAt = iso
      const snapshot = state.buildInputs[build.id] ?? {
        dataRevision: build.dataRevision,
        files: workspace.files,
        tests: workspace.tests,
      }
      build.packageCases = snapshot.tests.length
      build.tests = snapshot.tests.map((t) => ({
        index: t.index,
        isSample: t.isSample,
        source: t.source,
        status: 'ok',
        group: t.group,
        points: t.points,
        inputBytes: t.inputData.length,
        inputHead: t.inputData,
        answerBytes: 4,
        answerHead: '1 2\n',
        timeMs: 7,
        memoryKb: 3200,
      }))
      build.solutions = snapshot.files
        .filter((f) => f.kind === 'solution')
        .map((f) => ({
          name: f.name,
          language: f.language,
          expectedVerdict: f.expectedVerdict,
          actualVerdict: f.expectedVerdict || 'Accepted',
          matched: true,
          maxTimeMs: 7,
          maxMemoryKb: 3200,
          message: '模拟结果',
        }))
      if (snapshot.dataRevision === workspace.meta.dataRevision) {
        Object.assign(workspace.meta, {
          builtRevision: snapshot.dataRevision,
          testdataCases: snapshot.tests.length,
          testdataVersion: workspace.meta.testdataVersion + 1,
          testdataSha256: 'mock-' + build.id,
          lastBuiltAt: iso,
        })
        state.problemCandidateSamples[id] = build.tests
          .filter((test) => test.isSample)
          .map((test) => ({ input: test.inputHead ?? '', answer: test.answerHead ?? '' }))
      }
      refreshFlags()
    }
  }
  if (section === 'releases' && get) {
    const items = (state.problemReleases[id] ?? []).map((entry) => entry.release)
    return { items, total: items.length }
  }
  if (section === 'publish' && post) {
    if (!problem.permissions.publish) throw new MockError(403, '没有发布权限。')
    if (
      Number(body.revision) !== workspace.meta.packageRevision ||
      Number(body.artifactVersion) !== workspace.meta.testdataVersion ||
      workspace.meta.stale
    )
      throw new MockError(409, '工作副本或候选数据已变化，请刷新确认。')
    const language = text('language') || workspace.meta.statementLanguage
    const statement = workspace.statements.find((s) => s.language === language)
    if (!statement && language !== workspace.meta.statementLanguage)
      throw new MockError(400, '所选语言尚无已保存题面。')
    const markdown = statement
      ? renderStatement(statement, state.problemCandidateSamples[id])
      : draft.statementMd
    if (!markdown.trim()) throw new MockError(400, '请先填写题面。')
    const previous = state.problemReleases[id]?.[0]
    if (
      previous?.release.revision === workspace.meta.packageRevision &&
      previous.release.artifactVersion === workspace.meta.testdataVersion &&
      previous.release.language === language
    )
      return previous.release
    const version = problem.publishedVersion + 1
    Object.assign(problem, {
      difficulty: draft.difficulty,
      source: draft.source,
      timeLimitMs: draft.timeLimitMs,
      memoryLimitKb: draft.memoryLimitKb,
      judgeType: draft.judgeType,
      tags: [...draft.tags],
      publishedVersion: version,
      title: statement?.name || draft.title,
      statementMd: markdown,
      updatedAt: iso,
    })
    const release = {
      version,
      revision: workspace.meta.packageRevision,
      artifactVersion: workspace.meta.testdataVersion,
      language,
      sha256: workspace.meta.testdataSha256,
      caseCount: workspace.meta.testdataCases,
      createdAt: iso,
    }
    state.problemReleases[id] = [
      { release, problem: structuredClone(problem) },
      ...(state.problemReleases[id] ?? []),
    ]
    Object.assign(workspace.meta, {
      publishedVersion: version,
      publishedRevision: release.revision,
      publishedArtifactVersion: release.artifactVersion,
    })
    refreshFlags()
    return release
  }
  workspace.issues = [
    !workspace.statements.some(
      (s) => s.language === workspace.meta.statementLanguage && s.legend.trim(),
    )
      ? '请填写主语言题面。'
      : '',
    !workspace.tests.length ? '请添加至少一个测试点。' : '',
    !workspace.files.some((f) => f.kind === 'solution' && f.isActive)
      ? '请添加并启用一份标程。'
      : '',
  ].filter(Boolean)
  if (section === 'package' && get) return workspace
  if (section === 'statements') {
    if (get) return { items: workspace.statements, total: workspace.statements.length }
    const language = itemId.toLowerCase()
    if (!/^[a-z][a-z0-9-]{0,15}$/.test(language)) throw new MockError(400, '语言代码不合法。')
    if (del) {
      workspace.statements = workspace.statements.filter((s) => s.language !== language)
      touch(false)
      return { status: 'ok' }
    }
    const statement: DtoStatementResponse = {
      language,
      name: required('name'),
      legend: text('legend'),
      inputFormat: text('inputFormat'),
      outputFormat: text('outputFormat'),
      notes: text('notes'),
      scoring: text('scoring'),
      tutorial: text('tutorial'),
      updatedAt: iso,
    }
    if (post && action === 'preview')
      return { statementMd: renderStatement(statement, state.problemCandidateSamples[id]) }
    if (method === 'PUT') {
      workspace.statements = [
        ...workspace.statements.filter((s) => s.language !== language),
        statement,
      ]
      if (language === workspace.meta.statementLanguage) {
        draft.title = statement.name
        workspace.meta.title = statement.name
        draft.statementMd = renderStatement(statement, state.problemCandidateSamples[id])
      }
      touch(false)
      return statement
    }
  }
  if (section === 'files') {
    if (get && !itemId) return { items: workspace.files, total: workspace.files.length }
    if (get) {
      const file = workspace.files.find((f) => f.id === Number(itemId))
      if (!file) throw new MockError(404, '文件不存在。')
      return file
    }
    if (del) {
      workspace.files = workspace.files.filter((f) => f.id !== Number(itemId))
      touch()
      return { status: 'ok' }
    }
    if (method === 'PUT') {
      const name = required('name'),
        kind = required('kind') as DtoFileResponse['kind']
      if (
        ![
          'solution',
          'validator',
          'generator',
          'checker',
          'interactor',
          'grader',
          'header',
        ].includes(kind)
      )
        throw new MockError(400, '文件类型不合法。')
      const previous = workspace.files.find((f) => f.name === name && f.kind === kind)
      const file: DtoFileResponse = {
        id: previous?.id ?? Math.max(0, ...workspace.files.map((f) => f.id)) + 1,
        name,
        kind,
        language: required('language'),
        sourceCode: text('sourceCode'),
        expectedVerdict: text('expectedVerdict'),
        isActive: body.isActive !== false,
        updatedAt: iso,
      }
      workspace.files = [...workspace.files.filter((f) => f.id !== file.id), file]
      touch()
      return file
    }
  }
  if (section === 'tests') {
    if (get) return { items: workspace.tests, total: workspace.tests.length }
    if (del) {
      workspace.tests = workspace.tests.filter((t) => t.id !== Number(itemId))
      workspace.tests.forEach((t, i) => (t.index = i + 1))
      touch()
      return { status: 'ok' }
    }
    if (post && action === 'move') {
      const from = workspace.tests.findIndex((t) => t.id === Number(itemId)),
        to = Number(body.position) - 1
      if (from < 0 || to < 0 || to >= workspace.tests.length)
        throw new MockError(400, '测试点位置不合法。')
      workspace.tests.splice(to, 0, ...workspace.tests.splice(from, 1))
      workspace.tests.forEach((t, i) => (t.index = i + 1))
      touch()
      return { status: 'ok' }
    }
    if (post || method === 'PUT') {
      const previous = itemId ? workspace.tests.find((t) => t.id === Number(itemId)) : undefined
      if (itemId && !previous) throw new MockError(404, '测试点不存在。')
      const test: DtoTestResponse = {
        id: previous?.id ?? Math.max(0, ...workspace.tests.map((t) => t.id)) + 1,
        index: previous?.index ?? workspace.tests.length + 1,
        source: body.source === 'generator' ? 'generator' : 'manual',
        inputData: text('inputData'),
        generateCmd: text('generateCmd'),
        group: text('group'),
        description: text('description'),
        points: Number(body.points) || 0,
        isSample: body.isSample === true,
      }
      workspace.tests = [...workspace.tests.filter((t) => t.id !== test.id), test].sort(
        (a, b) => a.index - b.index,
      )
      touch()
      return test
    }
  }
  if (section === 'builds') {
    if (get && !itemId) return { items: build ? [build] : [], total: build ? 1 : 0 }
    if (get && build?.id === itemId) return build
    if (post && action === 'cancel') {
      if (!build || build.id !== itemId) throw new MockError(404, '构建不存在。')
      if (['queued', 'running'].includes(build.state)) {
        build.state = 'cancelled'
        build.finishedAt = iso
      }
      return build
    }
    if (post && !itemId) {
      if (build && ['queued', 'running'].includes(build.state))
        throw new MockError(409, '已有构建在进行。')
      if (workspace.issues.length) throw new MockError(400, workspace.issues.join(' '))
      workspace.latestBuild = {
        id: crypto.randomUUID(),
        problemId: id,
        state: 'queued',
        stage: 'queued',
        revision: workspace.meta.packageRevision,
        dataRevision: workspace.meta.dataRevision,
        attempt: 1,
        progressDone: 0,
        progressTotal: 5,
        packageCases: 0,
        createdAt: iso,
        log: '本地模拟构建：未执行程序。',
        tests: [],
        solutions: [],
      }
      state.buildInputs[workspace.latestBuild.id] = structuredClone({
        dataRevision: workspace.meta.dataRevision,
        files: workspace.files,
        tests: workspace.tests,
      })
      return workspace.latestBuild
    }
  }
  throw new MockError(501, '此出题接口尚未提供 mock，未向真实后端发送请求。')
}
