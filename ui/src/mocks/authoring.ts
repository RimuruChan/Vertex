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
      builtRevision: 0,
      stale: true,
      testdataVersion: 0,
      testdataCases: 0,
      testdataChecker: 'tokens',
      testdataSha256: '',
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

function renderStatement(statement: DtoStatementResponse) {
  return [
    [null, statement.legend],
    ['输入格式', statement.inputFormat],
    ['输出格式', statement.outputFormat],
    ['说明', statement.notes],
    ['计分方式', statement.scoring],
  ]
    .filter(([, value]) => value)
    .map(([title, value]) => (title ? `## ${title}\n\n${value}` : value))
    .join('\n\n')
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
    const items = state.problems.filter(
      (p) =>
        p.title.includes(String(params.keyword ?? '')) &&
        (!params.visibility || p.visibility === params.visibility),
    )
    const size = Number(params.size) || 20,
      page = Number(params.page) || 1
    return { items: items.slice((page - 1) * size, page * size), total: items.length }
  }
  if (!id && post) {
    const problem: DtoProblemResponse = {
      publicId: allocateReference(state, 'problems'),
      id: crypto.randomUUID(),
      title: required('title'),
      statementMd: text('statementMd'),
      difficulty: Number(body.difficulty) || 1,
      source: text('source'),
      timeLimitMs: Number(body.timeLimitMs) || 1000,
      memoryLimitKb: Number(body.memoryLimitKb) || 262144,
      visibility: text('visibility') || 'draft',
      judgeType: 'standard',
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
  const workspace = (state.workspaces[id] ??= initialWorkspace(problem))
  const touch = () => {
    workspace.meta.packageRevision++
    workspace.meta.stale = true
  }
  if (!section) {
    if (get) return problem
    if (del) {
      state.problems = state.problems.filter((p) => p.id !== id)
      delete state.workspaces[id]
      return { status: 'ok' }
    }
    if (method === 'PUT') {
      Object.assign(problem, {
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
      Object.assign(workspace.meta, {
        title: problem.title,
        timeLimitMs: problem.timeLimitMs,
        memoryLimitKb: problem.memoryLimitKb,
        visibility: problem.visibility,
      })
      touch()
      return problem
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
      build.packageCases = workspace.tests.length
      build.tests = workspace.tests.map((t) => ({
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
      build.solutions = workspace.files
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
      Object.assign(workspace.meta, {
        builtRevision: build.revision,
        stale: build.revision !== workspace.meta.packageRevision,
        testdataCases: workspace.tests.length,
        testdataVersion: workspace.meta.testdataVersion + 1,
        lastBuiltAt: iso,
      })
    }
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
      touch()
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
    if (post && action === 'preview') return { statementMd: renderStatement(statement) }
    if (method === 'PUT') {
      workspace.statements = [
        ...workspace.statements.filter((s) => s.language !== language),
        statement,
      ]
      if (language === workspace.meta.statementLanguage) {
        problem.title = statement.name
        workspace.meta.title = statement.name
        problem.statementMd = renderStatement(statement)
      }
      touch()
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
        attempt: 1,
        progressDone: 0,
        progressTotal: 5,
        packageCases: 0,
        createdAt: iso,
        log: '本地模拟构建：未执行程序。',
        tests: [],
        solutions: [],
      }
      return workspace.latestBuild
    }
  }
  throw new MockError(501, '此出题接口尚未提供 mock，未向真实后端发送请求。')
}
