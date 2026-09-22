import { describe, expect, it } from 'vitest'
import type { DomainImportReceipt, DomainTreeEntry } from '@/generated/api/model'
import { importReviewProblem, packageImpact, packageSourceKey } from './package-review'

const now = Date.parse('2026-09-22T12:00:00Z')
function receipt(): DomainImportReceipt {
  return {
    id: 'preview-1',
    applied: false,
    etag: 'copy-1',
    expiresAt: new Date(now + 60000).toISOString(),
    plan: {
      archiveHash: 'archive',
      canApply: true,
      fileCount: 1,
      format: 'vertex',
      scope: 'package',
      tree: { entries: [] },
      issues: [],
    },
  }
}
const context = () => ({
  receipt: receipt(),
  parametersMatch: true,
  etag: 'copy-1',
  canEdit: true,
  now,
})

describe('import receipt authorization', () => {
  it('allows valid plans with deferred build requirements without changing server import semantics', () => {
    const value = context()
    value.receipt.plan.issues = [
      { severity: 'blocking', code: 'build.required', message: '需要配置校验器' },
    ]
    expect(importReviewProblem(value)).toBeUndefined()
    value.receipt.plan.canApply = false
    expect(importReviewProblem(value)).toContain('修复')
  })
  it('rejects previously reviewed plans when the selected file or time-limit parameters changed', () => {
    expect(importReviewProblem({ ...context(), parametersMatch: false })).toContain(
      '文件或固定时限已变化',
    )
  })
  it('does not let a receipt replace a newer working copy', () => {
    expect(importReviewProblem({ ...context(), etag: 'copy-2' })).toContain('工作副本已变化')
  })
  it('rejects expired receipts at the exact expiration boundary', () => {
    const value = context()
    expect(importReviewProblem({ ...value, now: now + 59999 })).toBeUndefined()
    expect(importReviewProblem({ ...value, now: now + 60000 })).toContain('已过期')
    value.receipt.expiresAt = 'invalid'
    expect(importReviewProblem(value)).toContain('已过期')
  })
  it('blocks reapplication, read-only users, and unresolved merges even when the plan can apply', () => {
    const value = context()
    expect(importReviewProblem({ ...value, canEdit: false })).toContain('权限')
    expect(importReviewProblem({ ...value, mergeId: 'merge-1' })).toContain('冲突')
    value.receipt.applied = true
    expect(importReviewProblem(value)).toContain('已经应用')
  })
})

function entry(id: string, path = `${id}.md`, hash = id): DomainTreeEntry {
  return {
    id,
    path,
    kind: 'statement',
    attributes: { language: 'zh' },
    blob: { sha256: hash, bytes: 10 },
  }
}

it('reviews removals, content changes, and renames against the captured baseline', () => {
  const before = { entries: [entry('keep'), entry('remove'), entry('change'), entry('rename')] }
  const after = {
    entries: [
      entry('keep'),
      entry('change', 'change.md', 'new-content'),
      entry('rename', 'renamed.md'),
      entry('added'),
    ],
  }
  const impact = packageImpact(before, after)
  expect(impact.added.map((item) => item.entryId)).toEqual(['added'])
  expect(impact.modified.map((item) => item.entryId)).toEqual(['change', 'rename'])
  expect(impact.removed.map((item) => item.entryId)).toEqual(['remove'])
  expect(impact.modified[1].before?.path).toBe('rename.md')
  expect(impact.modified[1].after?.path).toBe('renamed.md')
  expect(before.entries).toHaveLength(4)
})

it('invalidates copy exports on material changes while immutable revision exports remain current', () => {
  expect(packageSourceKey('copy-1')).not.toBe(packageSourceKey('copy-2'))
  expect(packageSourceKey('copy-1', 7)).toBe(packageSourceKey('copy-2', 7))
  expect(packageSourceKey('copy-1', 7)).not.toBe(packageSourceKey('copy-1', 8))
  expect(packageSourceKey('copy-1', 7)).not.toBe(packageSourceKey('copy-1'))
})
