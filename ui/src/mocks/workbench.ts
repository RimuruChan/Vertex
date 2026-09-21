import { standardStatement } from '@/lib/statement-template'
import {
  mockGeneration,
  mockProgramSave,
  mockStructuredReview,
  mockStatementPreview,
} from './workbench-workflows'
import type { DomainGenerationInput, DomainProgramSaveInput } from '@/generated/api/model'
import type {
  DomainContentTree,
  DomainWorkingCopy,
  DomainContentCommit,
  DomainTreeEntry,
  DomainMergeSession,
  DomainContentConflict,
  DomainBlobRef,
  DomainMaterialView,
} from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'
import { mockCheckRequest, mockContentHash, type SavedMockCheck } from './workbench-checks'
import { workbenchLibrary } from './workbench-library'
import { mockPackageRequest, type SavedMockImport } from './workbench-packages'
import { problemPermissions } from './problem-permissions'
import {
  canonicalTree,
  contentChanges,
  defaultMetadata,
  emptyTree,
  isDocument,
  sameValue,
} from '@/lib/authoring-materials'

type StoredBlob = { bytes: number[]; owners: string[] }
type SavedCommit = {
  commit: DomainContentCommit
  tree: DomainContentTree
  requestId: string
  etag: string
}
export type MockWorkbench = {
  imports?: SavedMockImport[]
  checks?: SavedMockCheck[]
  releases?: import('@/generated/api/model').DomainCommitRelease[]
  initial: DomainContentTree
  commits: SavedCommit[]
  copies: Record<string, DomainWorkingCopy>
  merges: Record<string, DomainMergeSession>
  blobs: Record<string, StoredBlob>
  sequence: number
}
const token = () => crypto.randomUUID()
const fail = (status: number, message: string): never => {
  throw new MockError(status, message)
}

// The mock persists the same private-copy/shared-history distinction as the
// server. Blob references are deterministic fixture IDs, not security claims.
export function workbenchRequest(state: MockState, request: MockRequest, now: number): unknown {
  const { method, body = {}, params = {} } = request
  const parts = request.path.split('/').filter(Boolean)
  const id = parts[3],
    section = parts[4],
    child = parts[5],
    action = parts[6]
  const user = state.user ?? fail(401, '请先登录')
  if (!id && method === 'GET') return workbenchLibrary(state, params)
  const problem = state.problems.find((p) => p.id === id) ?? fail(404, '题目不存在')
  const permissions = problemPermissions(problem, user, state.scope, state.problemGrants?.[id])
  if (!permissions.readPackage) fail(403, '没有题目协作权限')
  if (
    method !== 'GET' &&
    !permissions.edit &&
    !(
      ['exports', 'statement-preview'].includes(section) &&
      method === 'POST' &&
      Number(body.revision) > 0
    )
  )
    fail(403, '没有题目编辑权限')
  if (section === 'visibility' && method === 'PUT') {
    if (!permissions.manageAccess) fail(403, '没有管理题目访问的权限')
    if (body.visibility !== 'private' && body.visibility !== 'public') fail(400, '无效可见性')
    if (body.expectedVisibility !== problem.visibility)
      fail(409, '题目可见性已变化，请重新读取后确认')
    problem.visibility = String(body.visibility)
    return { visibility: problem.visibility }
  }
  const stores = (state.workbenches ??= {})
  let store = stores[id]
  const time = new Date(now).toISOString()
  function put(bytes: number[]): DomainBlobRef {
    let digest = Object.keys(store.blobs).find((key) => sameValue(store.blobs[key].bytes, bytes))
    if (!digest) {
      digest = (++store.sequence).toString(16).padStart(64, '0')
      store.blobs[digest] = { bytes, owners: [] }
    }
    if (!store.blobs[digest].owners.includes(user.id)) store.blobs[digest].owners.push(user.id)
    return { sha256: digest, bytes: bytes.length }
  }
  const textBlob = (text: string) => put([...new TextEncoder().encode(text)])
  if (!store) {
    if (method === 'GET' && section === 'commits' && !child) return { items: [] }
    if (method !== 'POST' || section !== 'working-copy') fail(404, '尚未创建工作副本')
    store = stores[id] = {
      initial: emptyTree(),
      commits: [],
      copies: {},
      merges: {},
      blobs: {},
      sequence: 0,
    }
    const metadata = {
      ...defaultMetadata(problem.title),
      source: problem.source,
      difficulty: problem.difficulty,
      tags: [...problem.tags],
      timeLimitMs: problem.timeLimitMs,
      memoryLimitKb: problem.memoryLimitKb,
      judgeType: problem.judgeType === 'interactive' ? 'interactive' : 'normal',
    }
    store.initial = {
      entries: [
        {
          id: 'problem',
          path: 'vertex/problem.json',
          kind: 'metadata',
          attributes: { format: 'json' },
          blob: textBlob(JSON.stringify(metadata)),
        },
        {
          id: 'statement-zh',
          path: 'statement/problem.zh.md',
          kind: 'statement',
          attributes: { format: 'markdown', language: 'zh' },
          blob: textBlob(problem.statementMd || standardStatement(problem.title)),
        },
      ],
    }
  }
  const head = () => store.commits.at(-1)
  const headTree = () => head()?.tree ?? store.initial
  const getCopy = () => store.copies[user.id] ?? fail(404, '尚未创建工作副本')
  const checkedCopy = () => {
    const copy = getCopy()
    if (copy.etag !== body.etag) fail(409, '工作副本已变化，本地编辑已保留，请重新核对')
    return copy
  }
  const revision = (number: number) =>
    store.commits.find((item) => item.commit.revision === number) ?? fail(404, '提交不存在')
  const accessible = (digest: string) =>
    store.blobs[digest]?.owners.includes(user.id) ||
    store.commits.some((item) => item.tree.entries.some((e) => e.blob.sha256 === digest)) ||
    store.copies[user.id]?.tree.entries.some((e) => e.blob.sha256 === digest)
  function checkedTree(value: unknown): DomainContentTree {
    const tree = canonicalTree(value as DomainContentTree)
    const ids = new Set<string>(),
      paths = new Set<string>()
    for (const entry of tree.entries) {
      if (
        ids.has(entry.id) ||
        paths.has(entry.path.toLowerCase()) ||
        !entry.id ||
        !entry.path ||
        entry.path.includes('..') ||
        entry.path.includes('\\')
      )
        fail(400, '文件标识或路径重复/无效')
      if (
        !accessible(entry.blob.sha256) ||
        store.blobs[entry.blob.sha256].bytes.length !== entry.blob.bytes
      )
        fail(400, '材料内容不存在或不可访问')
      ids.add(entry.id)
      paths.add(entry.path.toLowerCase())
    }
    return tree
  }
  function save(copy: DomainWorkingCopy, tree: DomainContentTree) {
    if (copy.mergeId) fail(409, '请先解决冲突')
    tree = checkedTree(tree)
    if (!sameValue(copy.tree, tree)) {
      copy.tree = tree
      copy.etag = token()
      copy.updatedAt = time
    }
    copy.headRevision = head()?.commit.revision
    return copy
  }
  const decode = (entry: DomainTreeEntry) =>
    new TextDecoder().decode(new Uint8Array(store.blobs[entry.blob.sha256].bytes))
  if (section === 'imports' || section === 'exports')
    return mockPackageRequest({
      store,
      actor: user.id,
      request,
      now,
      copy: getCopy,
      put,
      save,
      validate: checkedTree,
    })
  if (section === 'origin' && method === 'GET') return { origin: state.problemOrigins?.[id] }
  if (['inspection', 'checks', 'releases'].includes(section))
    return mockCheckRequest({
      store,
      state,
      problem,
      permissions,
      request,
      now,
      actor: user.id,
      decode,
    })
  function updateTestOrder(
    tree: DomainContentTree,
    testId: string,
    remove: boolean,
  ): DomainContentTree {
    return {
      entries: tree.entries.map((entry) => {
        if (entry.id !== 'problem') return entry
        const metadata = JSON.parse(decode(entry))
        const order = (metadata.testOrder ?? []) as string[]
        metadata.testOrder = remove
          ? order.filter((id) => id !== testId)
          : order.includes(testId)
            ? order
            : [...order, testId]
        return { ...entry, blob: textBlob(JSON.stringify(metadata)) }
      }),
    }
  }
  function merge(copy: DomainWorkingCopy) {
    const base = copy.baseRevision ? revision(copy.baseRevision).tree : store.initial
    const b = new Map(base.entries.map((e) => [e.id, e])),
      l = new Map(copy.tree.entries.map((e) => [e.id, e])),
      r = new Map(headTree().entries.map((e) => [e.id, e]))
    const entries: DomainTreeEntry[] = [],
      conflicts: DomainContentConflict[] = []
    for (const id of [...new Set([...b.keys(), ...l.keys(), ...r.keys()])].sort()) {
      const bv = b.get(id),
        lv = l.get(id),
        rv = r.get(id)
      let selected = lv
      if (sameValue(lv, rv) || sameValue(bv, rv)) selected = lv
      else if (sameValue(bv, lv)) selected = rv
      else
        conflicts.push({
          entryId: id,
          field: 'entry',
          kind: !bv ? 'add-add' : !lv || !rv ? 'delete-modify' : 'both-modified',
          base: bv,
          local: lv,
          remote: rv,
        })
      if (selected) entries.push(structuredClone(selected))
    }
    const paths = new Map<string, DomainTreeEntry>()
    for (const entry of entries) {
      const other = paths.get(entry.path.toLowerCase())
      if (other)
        for (const value of [entry, other])
          conflicts.push({
            entryId: value.id,
            field: 'path',
            kind: 'path-collision',
            base: b.get(value.id),
            local: l.get(value.id),
            remote: r.get(value.id),
          })
      paths.set(entry.path.toLowerCase(), entry)
    }
    if (conflicts.length) {
      const session: DomainMergeSession = {
        id: token(),
        etag: token(),
        copyEtag: copy.etag,
        baseRevision: copy.baseRevision,
        remoteRevision: head()?.commit.revision,
        result: { tree: { entries }, conflicts },
      }
      store.merges[user.id] = session
      copy.mergeId = session.id
      return { copy, merge: session }
    }
    copy.tree = checkedTree({ entries })
    copy.baseRevision = head()?.commit.revision
    copy.headRevision = head()?.commit.revision
    return { copy }
  }
  if (section === 'blobs') {
    if (method === 'POST') return put(body.bytes as number[])
    if (!accessible(child)) fail(404, '文件不存在或不可访问')
    return { mockBlob: store.blobs[child].bytes }
  }
  if (section === 'working-copy') {
    if (method === 'POST' && !child) {
      if (!store.copies[user.id])
        store.copies[user.id] = {
          tree: structuredClone(headTree()),
          baseRevision: head()?.commit.revision,
          headRevision: head()?.commit.revision,
          etag: token(),
          updatedAt: time,
        }
      return getCopy()
    }
    if (method === 'GET' && !child) {
      getCopy().headRevision = head()?.commit.revision
      return getCopy()
    }
    const copy = checkedCopy()
    if (child === 'batch' && method === 'POST') {
      const tree = structuredClone(copy.tree),
        deleted = new Set<string>((body.deleteIds as string[]) ?? []),
        testIds = (body.testIds as string[]) ?? []
      if (
        deleted.size + testIds.length > 500 ||
        deleted.has('problem') ||
        [...deleted].some((id) => !tree.entries.some((entry) => entry.id === id))
      )
        fail(400, '删除选择无效')
      if (testIds.length && !body.patch) fail(400, '缺少批量修改内容')
      const patch = (body.patch as Record<string, unknown>) ?? {}
      for (const [key, max] of [
        ['timeLimitMs', 3600000],
        ['memoryLimitKb', 1073741824],
      ] as const) {
        const value = patch[key]
        if (
          value !== undefined &&
          (!Number.isSafeInteger(value) || Number(value) < 0 || Number(value) > max)
        )
          fail(400, '无效的批量限制')
      }
      if (
        patch.points !== undefined &&
        (!Number.isFinite(patch.points) || Number(patch.points) < 0)
      )
        fail(400, '无效分值')
      if (
        patch.group &&
        !tree.entries.some(
          (entry) => entry.id === patch.group && entry.kind === 'group' && !deleted.has(entry.id),
        )
      )
        fail(400, '分组不存在')
      for (const id of testIds) {
        const entry =
          tree.entries.find(
            (entry) => entry.id === id && entry.kind === 'test' && !deleted.has(id),
          ) ?? fail(400, '测试点选择无效')
        entry.blob = textBlob(JSON.stringify({ ...JSON.parse(decode(entry)), ...patch }))
      }
      tree.entries = tree.entries.filter((entry) => !deleted.has(entry.id))
      const metadata =
          tree.entries.find((entry) => entry.id === 'problem') ?? fail(400, '缺少基本设置'),
        value = JSON.parse(decode(metadata))
      value.testOrder = (value.testOrder as string[]).filter((id) => !deleted.has(id))
      if (body.testOrder) {
        const order = body.testOrder as string[],
          tests = tree.entries.filter((entry) => entry.kind === 'test')
        if (
          order.length !== tests.length ||
          new Set(order).size !== order.length ||
          order.some((id) => !tests.some((entry) => entry.id === id))
        )
          fail(400, '测试顺序必须包含全部测试点')
        value.testOrder = order
      }
      metadata.blob = textBlob(JSON.stringify(value))
      return save(copy, tree)
    }
    if (method === 'PUT' && !child) return save(copy, body.tree as DomainContentTree)
    if (child === 'entries') {
      const entryId = action
      if (method === 'DELETE') {
        if (entryId === 'problem') fail(400, '不能删除题目基本设置')
        const tree = { entries: copy.tree.entries.filter((e) => e.id !== entryId) }
        return save(
          copy,
          copy.tree.entries.find((e) => e.id === entryId)?.kind === 'test'
            ? updateTestOrder(tree, entryId, true)
            : tree,
        )
      }
      const existing = copy.tree.entries.find((e) => e.id === entryId)
      const entry = structuredClone(
        (parts[7] === 'text' ? existing : body.entry) as DomainTreeEntry,
      )
      if (!entry || entry.id !== entryId) fail(400, '材料标识不匹配')
      if (typeof body.text === 'string') {
        let text = body.text
        if (isDocument(entry.kind)) {
          let value: Record<string, unknown>
          try {
            value = JSON.parse(text)
          } catch {
            return fail(400, '材料不是合法的 JSON')
          }
          if (value.schemaVersion !== 1) fail(400, '未知材料版本')
          text = JSON.stringify(value)
        } else if (['source', 'statement'].includes(entry.kind)) text = text.replace(/\r\n?/g, '\n')
        entry.blob = textBlob(text)
      }
      const tree = { entries: [...copy.tree.entries.filter((e) => e.id !== entryId), entry] }
      return save(copy, entry.kind === 'test' ? updateTestOrder(tree, entryId, false) : tree)
    }
    if (child === 'update') {
      if (copy.mergeId) fail(409, '请先解决冲突')
      const result = merge(copy)
      copy.etag = token()
      if (result.merge) result.merge.copyEtag = copy.etag
      return result
    }
    if (child === 'discard' || child === 'restore') {
      copy.tree = structuredClone(
        child === 'restore' ? revision(Number(body.revision)).tree : headTree(),
      )
      if (child === 'discard') copy.baseRevision = head()?.commit.revision
      copy.etag = token()
      delete copy.mergeId
      delete store.merges[user.id]
      return copy
    }
  }
  if (section === 'commits') {
    if (method === 'GET') {
      if (child) {
        const item = revision(Number(child))
        return { commit: item.commit, tree: item.tree }
      }
      return {
        items: [...store.commits]
          .reverse()
          .filter((item) => !params.before || item.commit.revision < Number(params.before))
          .slice(0, Number(params.limit) || 30)
          .map((item) => item.commit),
      }
    }
    const prior = store.commits.find(
      (item) => item.commit.authorId === user.id && item.requestId === body.requestId,
    )
    if (prior) {
      if (prior.etag !== body.etag || prior.commit.message !== body.message)
        fail(400, '请求标识已用于其他提交')
      return { copy: getCopy(), commit: prior.commit }
    }
    const copy = checkedCopy()
    if (copy.mergeId) fail(409, '请先解决冲突')
    if (!String(body.message ?? '').trim()) fail(400, '填写提交说明')
    const originalToken = copy.etag
    const result = merge(copy)
    if (result.merge) return result
    if (head() && sameValue(copy.tree, headTree())) return { copy }
    const commit: DomainContentCommit = {
      revision: store.commits.length + 1,
      parentRevision: head()?.commit.revision,
      authorId: user.id,
      createdAt: time,
      message: String(body.message).trim(),
      treeHash: mockContentHash(copy.tree),
    }
    store.commits.push({
      commit,
      tree: structuredClone(copy.tree),
      requestId: String(body.requestId),
      etag: originalToken,
    })
    copy.baseRevision = commit.revision
    copy.headRevision = commit.revision
    copy.etag = token()
    copy.updatedAt = time
    return { copy, commit }
  }
  if (section === 'statement-preview' && method === 'POST')
    return mockStatementPreview(
      Number(body.revision) > 0 ? revision(Number(body.revision)).tree : checkedCopy().tree,
      String(body.entryId),
      String(body.content ?? ''),
      decode,
    )
  if (section === 'generation' && method === 'POST')
    return mockGeneration(
      { copy: checkedCopy(), decode, textBlob, save },
      body as unknown as DomainGenerationInput,
    )
  if (section === 'programs' && method === 'PUT')
    return mockProgramSave(
      { copy: checkedCopy(), decode, textBlob, save },
      body as unknown as DomainProgramSaveInput,
    )
  if (section === 'changes') {
    const target = params.revision ? revision(Number(params.revision)) : undefined
    const copy = target ? undefined : getCopy(),
      base = Number(params.from) || target?.commit.parentRevision || copy?.baseRevision
    return {
      etag: copy?.etag,
      fromRevision: base,
      toRevision: target?.commit.revision,
      changes: contentChanges(base ? revision(base).tree : emptyTree(), target?.tree ?? copy!.tree),
      review: mockStructuredReview(
        base ? revision(base).tree : emptyTree(),
        target?.tree ?? copy!.tree,
        decode,
      ),
    }
  }
  if (section === 'materials') {
    const tree = params.revision ? revision(Number(params.revision)).tree : getCopy().tree
    if (!child) {
      if (!params.revision && params.etag && params.etag !== getCopy().etag)
        fail(409, '工作副本已变化')
      const kind = String(params.kind),
        limit = Number(params.limit ?? 50)
      if (
        !['test', 'program', 'group', 'validation', 'generation'].includes(kind) ||
        !Number.isInteger(limit) ||
        limit < 1 ||
        limit > 100
      )
        fail(400, '无效分页选择')
      const metadata = tree.entries.find((entry) => entry.id === 'problem'),
        order = metadata ? (JSON.parse(decode(metadata)).testOrder as string[]) : []
      const entries = tree.entries
        .filter((entry) => entry.kind === kind)
        .sort((a, b) =>
          kind === 'test'
            ? (order.indexOf(a.id) < 0 ? Number.MAX_SAFE_INTEGER : order.indexOf(a.id)) -
                (order.indexOf(b.id) < 0 ? Number.MAX_SAFE_INTEGER : order.indexOf(b.id)) ||
              a.path.localeCompare(b.path)
            : a.path.localeCompare(b.path),
        )
      const start = params.after ? entries.findIndex((entry) => entry.id === params.after) + 1 : 0
      if (params.after && !start) fail(400, '分页游标已失效')
      const selected = entries.slice(start, start + limit)
      return {
        etag: params.revision ? undefined : getCopy().etag,
        revision: Number(params.revision) || undefined,
        total: entries.length,
        next: start + limit < entries.length ? selected.at(-1)?.id : undefined,
        items: selected.map((entry, index) => {
          try {
            return { entry, position: start + index + 1, [entry.kind]: JSON.parse(decode(entry)) }
          } catch {
            return { entry, position: start + index + 1, error: '材料文档无效' }
          }
        }),
      }
    }
    const entry = tree.entries.find((e) => e.id === child) ?? fail(404, '材料不存在')
    if (!isDocument(entry.kind)) fail(400, '此材料不是结构化文档')
    return { entry, [entry.kind]: JSON.parse(decode(entry)) } as DomainMaterialView
  }
  if (section === 'merges') {
    const session = store.merges[user.id]
    if (!session || session.id !== child) fail(404, '合并会话不存在')
    if (method === 'GET') return session
    if (body.etag !== session.etag) fail(409, '合并会话已变化')
    if (method === 'PUT') {
      const resolved = body.resolved as { entryId: string; field: string }[]
      session.result.tree = structuredClone(body.tree as DomainContentTree)
      session.result.conflicts = session.result.conflicts.filter(
        (item) => !resolved.some((key) => key.entryId === item.entryId && key.field === item.field),
      )
      session.etag = token()
      return session
    }
    if (action === 'complete') {
      if (session.result.conflicts.length) fail(409, '还有未解决的冲突')
      const copy = getCopy()
      if (copy.etag !== session.copyEtag) fail(409, '工作副本已变化')
      copy.tree = checkedTree(session.result.tree)
      copy.baseRevision = session.remoteRevision
      delete copy.mergeId
      delete store.merges[user.id]
      const next = merge(copy)
      copy.etag = token()
      if (next.merge) next.merge.copyEtag = copy.etag
      return copy
    }
  }
  return fail(501, '此出题操作尚未提供演示实现')
}
