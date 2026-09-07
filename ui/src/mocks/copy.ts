import type { DtoCopyResponse, DtoProblemResponse } from '@/generated/api/model'
import type { MockState } from './fixtures'
import { initialWorkspace, initialSamples } from './authoring'
import { allocateReference } from './references'
import { problemPermissions } from './problem-permissions'
import { mockCan } from './domain-policy'
import { MockError } from './errors'

// The root API resolves and authorizes each domain before passing its data graph.
export function copyProblem(
  source: MockState,
  target: MockState,
  body: Record<string, unknown>,
  now: number,
): DtoCopyResponse {
  const actor = target.user
  if (!actor) throw new MockError(401, '请先登录')
  if (!mockCan(target.scope, actor, 'problem.create'))
    throw new MockError(403, '目标域没有创建题目权限')
  const reference = String(body.sourceProblem ?? '')
      .trim()
      .toLowerCase(),
    version = Number(body.sourceVersion),
    note = typeof body.attribution === 'string' ? body.attribution.trim() : ''
  if (
    !Number.isSafeInteger(version) ||
    version <= 0 ||
    !note ||
    new TextEncoder().encode(note).length > 4096
  )
    throw new MockError(400, '请选择发布版本并填写复制说明')
  const parent = source.problems.find(
    (problem) => problem.id === reference || problem.publicId === reference,
  )
  if (!parent) throw new MockError(404, '源题目不存在')
  if (
    !problemPermissions(parent, source.user, source.scope, source.problemGrants?.[parent.id]).copy
  )
    throw new MockError(403, '没有源题目包复制权限')
  const release = source.problemReleases[parent.id]?.find(
    (item) => item.release.version === version,
  )
  if (!release) throw new MockError(404, '源发布版本不存在')
  if (!release.workspace && release.release.sha256 !== 'mock-initial')
    throw new MockError(409, '此旧演示版本没有完整包快照，请重新发布后复制')
  const attribution = [source.problemOrigins?.[parent.id]?.attribution, note]
    .filter(Boolean)
    .join('\n\n')
  if (new TextEncoder().encode(attribution).length > 8192)
    throw new MockError(400, '合并后的来源说明过长')
  const iso = new Date(now).toISOString()
  const problem: DtoProblemResponse = {
    ...structuredClone(release.problem),
    id: crypto.randomUUID(),
    publicId: allocateReference(target, 'problems'),
    domainId: target.scope!.id,
    ownerId: actor.id,
    ownerName: actor.username,
    authorId: actor.id,
    visibility: 'draft',
    publishedVersion: 0,
    submissionCount: 0,
    acceptedCount: 0,
    solvedUserCount: 0,
    userStatus: 'none',
    createdAt: iso,
    updatedAt: iso,
  }
  problem.permissions = problemPermissions(problem, actor, target.scope)
  const workspace = structuredClone(release.workspace ?? initialWorkspace(release.problem))
  workspace.latestBuild = undefined
  Object.assign(workspace.meta, {
    problemId: problem.id,
    problemPublicId: problem.publicId,
    title: problem.title,
    visibility: 'draft',
    timeLimitMs: problem.timeLimitMs,
    memoryLimitKb: problem.memoryLimitKb,
    judgeType: problem.judgeType,
    statementLanguage: release.release.language,
    packageRevision: 1,
    dataRevision: 1,
    builtRevision: 1,
    publishedVersion: 0,
    publishedRevision: -1,
    publishedArtifactVersion: 0,
    unpublishedChanges: true,
    stale: false,
    testdataVersion: 1,
    testdataCases: release.release.caseCount,
    testdataSha256: release.release.sha256,
    lastBuiltAt: undefined,
    canEdit: true,
    canPublish: true,
  })
  workspace.files = workspace.files.map((file, index) => ({ ...file, id: index + 1 }))
  workspace.tests = workspace.tests.map((test, index) => ({ ...test, id: index + 1 }))
  const origin = {
    sourceDomainId: source.scope!.id,
    sourceDomainSlug: source.scope!.slug,
    sourceProblemId: parent.id,
    sourceProblemNumber: parent.publicId,
    sourceVersion: version,
    sourceTitle: release.problem.title,
    sourceSha256: release.release.sha256,
    attribution,
    copiedBy: actor.id,
    copiedAt: iso,
  }
  target.problems.unshift(problem)
  target.problemDrafts[problem.id] = structuredClone(problem)
  target.workspaces[problem.id] = workspace
  target.problemCandidateSamples[problem.id] = structuredClone(
    release.samples ?? initialSamples(release.problem),
  )
  ;(target.problemOrigins ??= {})[problem.id] = origin
  return {
    problemId: problem.id,
    problemPublicId: problem.publicId,
    domainId: target.scope!.id,
    domainSlug: target.scope!.slug,
    origin,
  }
}
