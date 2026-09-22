import { describe, expect, it } from 'vitest'
import type {
  DomainCheckRun,
  DomainCommitRelease,
  DomainMaterialInspection,
} from '@/generated/api/model'
import { checkMatchesRelease, selectionIsCurrentRelease } from './release-readiness'

const inspection: DomainMaterialInspection = {
  canBuild: true,
  revision: 3,
  dataHash: 'data-current',
  policyVersion: 'policy-current',
  treeHash: 'tree-new-wording',
  issues: [],
  publicationIssues: [],
  testCount: 1,
  sampleCount: 1,
  programCount: 1,
  validationCount: 0,
}
const check: DomainCheckRun = {
  id: 'check',
  revision: 2,
  dataHash: 'data-current',
  policyVersion: 'policy-current',
  treeHash: 'tree-old-wording',
  createdAt: '',
  state: 'succeeded',
  stage: 'done',
  packageCases: 1,
  progressDone: 1,
  progressTotal: 1,
}
const release: DomainCommitRelease = {
  version: 4,
  revision: 3,
  checkId: 'check',
  language: 'zh',
  createdAt: '',
  toolchainKey: '',
  treeHash: 'tree-new-wording',
}

describe('release selection matching', () => {
  it('allows the same checked data across wording-only revisions', () => {
    expect(checkMatchesRelease(check, inspection)).toBe(true)
  })
  it('rejects missing inspection, changed data and outdated check policy', () => {
    expect(checkMatchesRelease(check, undefined)).toBe(false)
    expect(checkMatchesRelease(check, { ...inspection, dataHash: undefined })).toBe(false)
    expect(checkMatchesRelease(check, { ...inspection, dataHash: 'new-data' })).toBe(false)
    expect(checkMatchesRelease(check, { ...inspection, policyVersion: 'new-policy' })).toBe(false)
  })
  it('only marks the exact currently published selection as published', () => {
    expect(selectionIsCurrentRelease(release, 4, 3, 'check', 'zh')).toBe(true)
    expect(selectionIsCurrentRelease(release, 5, 3, 'check', 'zh')).toBe(false)
    expect(selectionIsCurrentRelease(release, 4, 4, 'check', 'zh')).toBe(false)
    expect(selectionIsCurrentRelease(release, 4, 3, 'other-check', 'zh')).toBe(false)
    expect(selectionIsCurrentRelease(release, 4, 3, 'check', 'en')).toBe(false)
    expect(selectionIsCurrentRelease(release, 4, 3, undefined, 'zh')).toBe(false)
  })
})
