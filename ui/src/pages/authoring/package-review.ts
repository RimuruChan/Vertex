import type { DomainContentTree, DomainImportReceipt } from '@/generated/api/model'
import { contentChanges } from '@/lib/authoring-materials'

export function importReviewProblem({
  receipt,
  parametersMatch,
  etag,
  mergeId,
  canEdit,
  now,
}: {
  receipt: DomainImportReceipt
  parametersMatch: boolean
  etag: string
  mergeId?: string
  canEdit: boolean
  now: number
}) {
  if (!canEdit) return '当前没有编辑权限，不能应用题包。'
  if (mergeId) return '请先解决工作副本的合并冲突，再重新预检。'
  if (!parametersMatch) return '题包文件或固定时限已变化，请重新预检。'
  if (receipt.applied) return '这份预检已经应用，请重新预检后再导入。'
  if (receipt.etag !== etag) return '工作副本已变化，之前的影响清单已过期，请重新预检。'
  const expires = Date.parse(receipt.expiresAt)
  if (!Number.isFinite(expires) || expires <= now) return '这份预检已过期，请重新预检。'
  if (!receipt.plan.canApply) return '题包暂时无法应用，请修复下方错误后重新预检。'
}

export function packageImpact(before: DomainContentTree, after: DomainContentTree) {
  const changes = contentChanges(before, after)
  return {
    added: changes.filter((item) => item.kind === 'added'),
    modified: changes.filter((item) => item.kind === 'modified' || item.kind === 'renamed'),
    removed: changes.filter((item) => item.kind === 'deleted'),
  }
}

export function packageSourceKey(etag: string, revision?: number) {
  return revision ? `revision:${revision}` : `copy:${etag}`
}
