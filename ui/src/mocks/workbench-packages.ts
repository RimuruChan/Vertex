import type {
  DomainBlobRef,
  DomainContentTree,
  DomainImportReceipt,
  DomainPackageExport,
  DomainTreeEntry,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { defaultTest, sameValue } from '@/lib/authoring-materials'
import type { MockRequest } from './api'
import type { MockWorkbench } from './workbench'
import type { PreparedArchive } from './package-archive'
import { MockError } from './errors'

export type SavedMockImport = { actor: string; receipt: DomainImportReceipt }
const fail = (status: number, message: string): never => {
  throw new MockError(status, message)
}

export function mockPackageRequest(context: {
  store: MockWorkbench
  actor: string
  request: MockRequest
  now: number
  copy: () => DomainWorkingCopy
  put: (bytes: number[]) => DomainBlobRef
  save: (copy: DomainWorkingCopy, tree: DomainContentTree) => DomainWorkingCopy
  validate: (tree: DomainContentTree) => DomainContentTree
}): unknown {
  const { store, actor, request, now, put, save, validate } = context
  const { method, body = {} } = request
  const [, , , , section, id, action] = request.path.split('/').filter(Boolean)
  const imports = (store.imports ??= [])
  if (section === 'exports' && method === 'POST') {
    if (body.format !== 'vertex' && body.format !== 'luogu-data')
      return fail(501, '演示模式提供 Vertex 原生归档导出；标准格式请使用真实后端。')
    const prepared = body.mockExport as
      { bytes: number[]; tree: DomainContentTree; etag?: string } | undefined
    if (!prepared) return fail(400, '缺少已准备的导出内容')
    const revision = Number(body.revision)
    const tree = revision
      ? store.commits.find((item) => item.commit.revision === revision)?.tree
      : context.copy().tree
    if (!tree) return fail(404, '提交不存在')
    if (!sameValue(tree, prepared.tree) || (!revision && context.copy().etag !== prepared.etag))
      return fail(409, '工作副本已变化，请重新导出')
    return {
      file: put(prepared.bytes),
      filename:
        body.format === 'luogu-data'
          ? 'problem-data.zip'
          : `vertex-${revision ? 'r' + revision : 'draft'}.zip`,
      format: String(body.format),
      issues:
        body.format === 'luogu-data'
          ? [
              {
                severity: 'warning',
                code: 'data.only',
                message: '仅含输入与答案，不包含题面、程序、样例标记或计分规则。',
              },
            ]
          : [],
    } satisfies DomainPackageExport
  }
  if (section !== 'imports') return fail(404, '题包操作不存在')
  if (id) {
    const record = imports.find((item) => item.actor === actor && item.receipt.id === id)
    if (!record) return fail(404, '预检不存在')
    if (method === 'GET') return record.receipt
    if (method === 'POST' && action === 'apply') {
      const copy = context.copy()
      if (record.receipt.applied) return copy
      if (copy.etag !== body.etag || record.receipt.etag !== body.etag)
        return fail(409, '工作副本已变化，请重新预检；原内容已保留')
      if (Date.parse(record.receipt.expiresAt) <= now) return fail(400, '预检已过期，请重新上传')
      const next = save(copy, record.receipt.plan.tree)
      record.receipt.applied = true
      return next
    }
    return fail(404, '预检操作不存在')
  }
  if (method !== 'POST') return fail(404, '题包操作不存在')
  const copy = context.copy()
  if (copy.mergeId) return fail(409, '请先解决冲突')
  if (copy.etag !== body.etag) return fail(409, '工作副本已变化，请重新预检')
  const archive = body.mockArchive as PreparedArchive | undefined
  if (!archive) return fail(400, '缺少已解压的题包')
  let tree: DomainContentTree
  if (archive.format === 'vertex' && archive.tree) {
    tree = {
      entries: archive.tree.entries.map((entry) => ({
        ...entry,
        blob: put(archive.files['blobs/' + entry.blob.sha256]),
      })),
    }
  } else if (archive.format === 'luogu-data') {
    const files = archive.files,
      used = new Set<string>()
    const inputs = Object.keys(files)
      .filter((name) => name.endsWith('.in'))
      .sort((a, b) => {
        const left = (a.match(/\d+/)?.[0] ?? '').replace(/^0+/, ''),
          right = (b.match(/\d+/)?.[0] ?? '').replace(/^0+/, '')
        return left.length - right.length || left.localeCompare(right) || a.localeCompare(b)
      })
    if (!inputs.length) return fail(400, '数据包需要成对的 .in/.out 或 .in/.ans')
    // Validate all pairs before installing anything in the working tree.
    for (const input of inputs) {
      const stem = input.slice(0, -3),
        answers = [stem + '.out', stem + '.ans'].filter((name) => name in files)
      if (answers.length !== 1) return fail(400, '每个输入需要唯一答案：' + input)
      used.add(input)
      used.add(answers[0])
    }
    if (used.size !== Object.keys(files).length) return fail(400, '数据包存在未配对的文件')
    tree = structuredClone(copy.tree)
    const metadata = tree.entries.find((entry) => entry.id === 'problem')
    if (!metadata) return fail(400, '先创建题目基本设置')
    const document = JSON.parse(
      new TextDecoder().decode(new Uint8Array(store.blobs[metadata.blob.sha256].bytes)),
    )
    const order: string[] = [...(document.testOrder ?? [])]
    function upsert(path: string, kind: string, bytes: number[], id?: string) {
      const found = tree.entries.find((entry) => entry.path === path)
      if (found && found.kind !== kind) return fail(400, '导入路径与现有材料冲突：' + path)
      const entry: DomainTreeEntry = {
        id: found?.id ?? id ?? 'import-' + crypto.randomUUID(),
        path,
        kind,
        attributes: kind === 'test' ? { format: 'json' } : {},
        blob: put(bytes),
      }
      if (found) Object.assign(found, entry)
      else tree.entries.push(entry)
      return entry.id
    }
    for (const input of inputs) {
      const stem = input.slice(0, -3),
        answer = stem + (stem + '.ans' in files ? '.ans' : '.out')
      const inputID = upsert('data/imported/' + input, 'input', files[input])
      const answerID = upsert('data/imported/' + stem + '.ans', 'answer', files[answer])
      const path = 'vertex/tests/imported-' + stem + '.json'
      const existing = tree.entries.find((entry) => entry.path === path)
      const prior = existing
        ? JSON.parse(
            new TextDecoder().decode(new Uint8Array(store.blobs[existing.blob.sha256].bytes)),
          )
        : defaultTest()
      const test = {
        ...prior,
        name: stem,
        input: { kind: 'file', entry: inputID, generator: '', arguments: [] },
        answer: { kind: 'file', entry: answerID, solution: '' },
      }
      const testID = upsert(path, 'test', [...new TextEncoder().encode(JSON.stringify(test))])
      if (!order.includes(testID)) order.push(testID)
    }
    metadata.blob = put([
      ...new TextEncoder().encode(JSON.stringify({ ...document, testOrder: order })),
    ])
  } else return fail(400, '未知演示题包格式')
  tree = validate(tree)
  const receipt: DomainImportReceipt = {
    id: crypto.randomUUID(),
    etag: copy.etag,
    expiresAt: new Date(now + 3600000).toISOString(),
    applied: false,
    plan: {
      format: archive.format,
      scope: archive.format === 'vertex' ? 'package' : 'data',
      archiveHash: archive.hash,
      tree,
      issues: [],
      fileCount: Object.keys(archive.files).length,
      canApply: true,
    },
  }
  imports.push({ actor, receipt })
  return receipt
}
