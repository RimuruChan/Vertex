import type { DomainLibraryItem, DomainLibraryPage } from '@/generated/api/model'
import type { MockState } from './fixtures'
import { MockError } from './errors'
import { problemPermissions } from './problem-permissions'
import { sameValue } from '@/lib/authoring-materials'
import { mockContentHash } from './workbench-checks'

export function workbenchLibrary(
  state: MockState,
  params: Record<string, unknown>,
): DomainLibraryPage {
  const user = state.user
  if (!user) throw new MockError(401, '请先登录')
  const page = Number(params.page ?? 1),
    size = Number(params.size ?? 20)
  if (!Number.isInteger(page) || page < 1 || !Number.isInteger(size) || size < 1 || size > 100)
    throw new MockError(400, '无效分页')
  const keyword = String(params.keyword ?? '')
    .trim()
    .toLowerCase()
  const items: DomainLibraryItem[] = []
  for (const problem of state.problems) {
    const permissions = problemPermissions(
      problem,
      user,
      state.scope,
      state.problemGrants?.[problem.id],
    )
    if (!permissions.readPackage) continue
    const store = state.workbenches?.[problem.id],
      copy = store?.copies[user.id],
      head = store?.commits.at(-1)
    const base =
      store?.commits.find((item) => item.commit.revision === copy?.baseRevision)?.tree ??
      store?.initial
    const tree = copy?.tree ?? head?.tree ?? store?.initial
    const entry = tree?.entries.find((entry) => entry.id === 'problem' && entry.kind === 'metadata')
    let title = problem.title,
      source = problem.source
    if (entry && store) {
      try {
        const meta = JSON.parse(
          new TextDecoder().decode(new Uint8Array(store.blobs[entry.blob.sha256].bytes)),
        )
        if (typeof meta.title === 'string' && meta.title) title = meta.title
        if (typeof meta.source === 'string') source = meta.source
      } catch {
        /* Malformed drafts retain their original list label. */
      }
    }
    const check = store?.checks?.find(
      (item) =>
        item.actor === user.id ||
        item.run.revision ||
        store.commits.some((commit) => sameValue(commit.tree, item.tree)),
    )
    const release = store?.releases?.find((item) => item.version === problem.publishedVersion)
    const item: DomainLibraryItem = {
      id: problem.id,
      title,
      source,
      visibility: problem.visibility,
      ownerName: problem.ownerName,
      canEdit: permissions.edit,
      canPublish: permissions.publish,
      hasCopy: Boolean(copy),
      hasChanges: Boolean(copy && !sameValue(copy.tree, base)),
      hasConflict: Boolean(copy?.mergeId),
      baseRevision: copy?.baseRevision ?? 0,
      headRevision: head?.commit.revision ?? 0,
      publishedVersion: problem.publishedVersion,
      publishedRevision: release?.revision ?? 0,
      checkId: check?.run.id,
      checkState: check?.run.state,
      checkMatches: Boolean(check && tree && check.run.treeHash === mockContentHash(tree)),
      updatedAt: [problem.updatedAt, copy?.updatedAt, head?.commit.createdAt]
        .filter((value): value is string => Boolean(value))
        .sort()
        .at(-1)!,
    }
    if (params.visibility && item.visibility !== params.visibility) continue
    if (
      keyword &&
      !title.toLowerCase().includes(keyword) &&
      !source.toLowerCase().includes(keyword) &&
      problem.publicId !== keyword
    )
      continue
    if (
      (params.status === 'changes' && !item.hasChanges) ||
      (params.status === 'conflicts' && !item.hasConflict) ||
      (params.status === 'unpublished' && item.publishedVersion !== 0)
    )
      continue
    items.push(item)
  }
  items.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt) || b.id.localeCompare(a.id))
  return { items: items.slice((page - 1) * size, page * size), total: items.length }
}
