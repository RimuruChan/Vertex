import type {
  DomainContentTree,
  DomainWorkingCopy,
  DtoProblemResponse,
} from '@/generated/api/model'
import type { createMockAPI, MockRequest } from './api'
import { createDemoArchive, createDemoDataArchive, prepareDemoArchive } from './package-archive'
import { MockError } from './errors'

// Browser-only preparation is asynchronous; committing demo state remains in
// the synchronous API so it can recheck current permissions and copy tokens.
export async function preparePackageRequest(
  api: ReturnType<typeof createMockAPI>,
  request: MockRequest,
): Promise<MockRequest> {
  const route = request.path.match(
    /^(\/api\/domains\/[^/]+)\/authoring\/problems\/([^/]+)\/(imports|exports)$/,
  )
  if (request.method !== 'POST' || !route) return request
  const [, domain, id, action] = route,
    body = request.body ?? {},
    actor = api.state.user?.id
  const resource = api.handle({
    method: 'GET',
    path: `${domain}/admin/problems/${id}`,
  }) as DtoProblemResponse
  if (action === 'imports' && !resource.permissions.edit)
    throw new MockError(403, '没有题目编辑权限')
  const path = `${domain}/authoring/problems/${id}`
  let prepared: Record<string, unknown>
  if (action === 'imports') {
    if (!(body.file instanceof Blob)) throw new MockError(400, '请选择 ZIP 文件')
    prepared = { ...body, file: undefined, mockArchive: await prepareDemoArchive(body.file) }
  } else {
    if (body.format !== 'vertex' && body.format !== 'luogu-data')
      throw new MockError(501, '演示模式提供 Vertex 原生归档导出；标准格式请使用真实后端。')
    const revision = Number(body.revision)
    const selected = api.handle({
      method: 'GET',
      path: path + (revision ? '/commits/' + revision : '/working-copy'),
    }) as { tree: DomainContentTree } & Partial<DomainWorkingCopy>
    const bytes = await (body.format === 'luogu-data' ? createDemoDataArchive : createDemoArchive)(
      selected.tree,
      (entry) =>
        (
          api.handle({ method: 'GET', path: path + '/blobs/' + entry.blob.sha256 }) as {
            mockBlob: number[]
          }
        ).mockBlob,
    )
    prepared = { ...body, mockExport: { bytes, tree: selected.tree, etag: selected.etag } }
  }
  if (!actor || actor !== api.state.user?.id) throw new MockError(409, '演示身份已变化，请重新操作')
  return { ...request, body: prepared }
}
