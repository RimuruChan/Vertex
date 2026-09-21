import type { DtoCopyResponse, DtoProblemResponse } from './models'
import type { MockState } from './fixtures'
import { MockError } from './errors'
import { mockCan } from './domain-policy'
import { problemPermissions } from './problem-permissions'
import { allocateReference } from './references'
import { mockContentHash } from './workbench-checks'
import type { MockWorkbench } from './workbench'

export function copyWorkbenchRelease(
  source: MockState,
  target: MockState,
  body: Record<string, unknown>,
  now: number,
): DtoCopyResponse {
  const actor = target.user
  if (!actor) throw new MockError(401, '请先登录')
  if (!mockCan(target.scope, actor, 'problem.create'))
    throw new MockError(403, '目标域没有创建题目权限')
  const reference = String(body.sourceProblem ?? ''),
    version = Number(body.sourceVersion),
    note = String(body.attribution ?? '').trim()
  if (
    !/^\d+$/.test(reference) ||
    !Number.isSafeInteger(version) ||
    version < 1 ||
    !note ||
    new TextEncoder().encode(note).length > 4096
  )
    throw new MockError(400, '选择发布版本并填写复制说明')
  const parent = source.problems.find((item) => item.publicId === reference)
  if (!parent) throw new MockError(404, '来源题目不存在')
  if (
    !problemPermissions(parent, source.user, source.scope, source.problemGrants?.[parent.id]).copy
  )
    throw new MockError(403, '没有来源题目包权限')
  const store = source.workbenches?.[parent.id],
    release = store?.releases?.find((item) => item.version === version)
  const committed = store?.commits.find((item) => item.commit.revision === release?.revision),
    check = store?.checks?.find((item) => item.run.id === release?.checkId)
  const projection = source.problemReleases[parent.id]?.find(
    (item) => item.release.version === version,
  )?.problem
  if (!store || !release || !committed || !check || check.run.state !== 'succeeded' || !projection)
    throw new MockError(409, '请先在新工作台提交、检查并发布此题目')
  const attribution = [source.problemOrigins?.[parent.id]?.attribution, note]
    .filter(Boolean)
    .join('\n\n')
  if (new TextEncoder().encode(attribution).length > 8192) throw new MockError(400, '来源说明过长')
  const time = new Date(now).toISOString()
  const problem: DtoProblemResponse = {
    ...structuredClone(projection),
    id: crypto.randomUUID(),
    publicId: allocateReference(target, 'problems'),
    domainId: target.scope!.id,
    ownerId: actor.id,
    ownerName: actor.username,
    authorId: actor.id,
    visibility: 'private',
    publishedVersion: 0,
    submissionCount: 0,
    acceptedCount: 0,
    solvedUserCount: 0,
    userStatus: 'none',
    createdAt: time,
    updatedAt: time,
  }
  problem.permissions = problemPermissions(problem, actor, target.scope)
  const tree = structuredClone(committed.tree)
  const blobs: MockWorkbench['blobs'] = {}
  for (const entry of tree.entries) {
    const data = store.blobs[entry.blob.sha256]
    if (!data) throw new MockError(409, '来源材料不完整')
    blobs[entry.blob.sha256] = { bytes: [...data.bytes], owners: [actor.id] }
  }
  const copied: MockWorkbench = {
    initial: tree,
    commits: [],
    copies: {
      [actor.id]: { etag: crypto.randomUUID(), tree: structuredClone(tree), updatedAt: time },
    },
    merges: {},
    blobs,
    sequence: store.sequence,
    releases: [],
    checks: [
      {
        actor: actor.id,
        tree: structuredClone(tree),
        steps: 3,
        run: {
          ...structuredClone(check.run),
          id: crypto.randomUUID(),
          revision: undefined,
          matchingRevision: undefined,
          treeHash: mockContentHash(tree),
          state: 'succeeded',
          stage: 'copied',
          createdAt: time,
          startedAt: undefined,
          finishedAt: time,
          log: '本地演示：复用来源已发布版本的验证材料，未执行程序。',
          solutions: [],
          tests: check.run.tests?.map((item) => ({
            ...item,
            inputHead: undefined,
            answerHead: undefined,
            message: undefined,
          })),
        },
      },
    ],
  }
  const origin = {
    sourceDomainId: source.scope!.id,
    sourceDomainSlug: source.scope!.slug,
    sourceProblemNumber: reference,
    sourceVersion: version,
    sourceTitle: projection.title,
    sourceSha256: release.treeHash,
    attribution,
    copiedBy: actor.id,
    copiedAt: time,
  }
  target.problems.unshift(problem)
  ;(target.workbenches ??= {})[problem.id] = copied
  ;(target.problemOrigins ??= {})[problem.id] = origin
  return {
    problemId: problem.id,
    problemPublicId: problem.publicId,
    domainId: target.scope!.id,
    domainSlug: target.scope!.slug,
    origin,
  }
}
