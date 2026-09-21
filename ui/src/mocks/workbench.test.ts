import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser } from './identities'
import { defaultTest } from '@/lib/authoring-materials'
import type {
  DomainWorkingCopy,
  DomainCommitOutcome,
  DomainContentComparison,
  DomainMaterialView,
  DomainBlobRef,
  DomainMergeSession,
  DomainMaterialPage,
} from '@/generated/api/model'

function setup() {
  const api = createMockAPI(createFixtures())
  api.state.user = { ...adminUser }
  return api
}
const path = '/api/domains/official/authoring/problems/1000'
describe('authoring working copy mock', () => {
  it('keeps visibility governance separate from private edits and checks its authority', () => {
    const api = setup()
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'editor' },
    })
    const copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    api.handle({
      method: 'PUT',
      path: path + '/visibility',
      body: { visibility: 'private', expectedVisibility: 'public' },
    })
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/visibility',
        body: { visibility: 'public', expectedVisibility: 'private' },
      }),
    ).toThrow('管理题目访问')
    api.state.user = { ...adminUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/visibility',
        body: { visibility: 'private', expectedVisibility: 'public' },
      }),
    ).toThrow('可见性已变化')
    expect(
      (api.handle({ method: 'GET', path: path + '/working-copy' }) as DomainWorkingCopy).etag,
    ).toBe(copy.etag)
    api.handle({
      method: 'PUT',
      path: path + '/visibility',
      body: { visibility: 'public', expectedVisibility: 'private' },
    })
  })
  it('pages in test order and applies a batch atomically without implicit commits', () => {
    const api = setup()
    let copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    for (let index = 0; index < 32; index++)
      copy = api.handle({
        method: 'PUT',
        path: path + `/working-copy/entries/test-${index}`,
        body: {
          etag: copy.etag,
          entry: {
            id: `test-${index}`,
            kind: 'test',
            path: `vertex/tests/${index}.json`,
            attributes: { format: 'json' },
          },
          text: JSON.stringify({ ...defaultTest(), name: `测试 ${index}` }),
        },
      }) as DomainWorkingCopy
    const first = api.handle({
      method: 'GET',
      path: path + '/materials',
      params: { kind: 'test', limit: 25, etag: copy.etag },
    }) as DomainMaterialPage
    expect(first.items).toHaveLength(25)
    expect(first.next).toBe('test-24')
    expect(first.total).toBe(32)
    const second = api.handle({
      method: 'GET',
      path: path + '/materials',
      params: { kind: 'test', limit: 25, after: first.next, etag: copy.etag },
    }) as DomainMaterialPage
    expect(second.items).toHaveLength(7)
    expect(second.items[0].position).toBe(26)
    expect(() =>
      api.handle({
        method: 'POST',
        path: path + '/working-copy/batch',
        body: { etag: copy.etag, testIds: ['test-0', 'missing'], patch: { isSample: true } },
      }),
    ).toThrow('测试点选择无效')
    const untouched = api.handle({
      method: 'GET',
      path: path + '/working-copy',
    }) as DomainWorkingCopy
    expect(untouched.etag).toBe(copy.etag)
    expect(
      (api.handle({ method: 'GET', path: path + '/materials/test-0' }) as DomainMaterialView).test
        ?.isSample,
    ).toBe(false)
    const oldToken = copy.etag
    copy = api.handle({
      method: 'POST',
      path: path + '/working-copy/batch',
      body: {
        etag: copy.etag,
        testIds: ['test-0', 'test-1'],
        patch: { isSample: true, timeLimitMs: 1500, memoryLimitKb: 131072 },
        testOrder: Array.from({ length: 32 }, (_, index) => `test-${31 - index}`),
      },
    }) as DomainWorkingCopy
    expect(
      (api.handle({ method: 'GET', path: path + '/materials/test-0' }) as DomainMaterialView).test,
    ).toMatchObject({ timeLimitMs: 1500, memoryLimitKb: 131072 })
    expect(() =>
      api.handle({
        method: 'POST',
        path: path + '/working-copy/batch',
        body: { etag: oldToken, deleteIds: ['test-0'] },
      }),
    ).toThrow('工作副本已变化')
    const reordered = api.handle({
      method: 'GET',
      path: path + '/materials',
      params: { kind: 'test', limit: 1, etag: copy.etag },
    }) as DomainMaterialPage
    expect(reordered.items[0].entry.id).toBe('test-31')
    api.handle({
      method: 'POST',
      path: path + '/working-copy/batch',
      body: { etag: copy.etag, deleteIds: ['test-0', 'test-1'] },
    })
    expect(
      (api.handle({ method: 'GET', path: path + '/materials/problem' }) as DomainMaterialView)
        .metadata?.testOrder,
    ).toHaveLength(30)
    expect(api.handle({ method: 'GET', path: path + '/commits' })).toEqual({ items: [] })
  })
  it('persists a manually edited conflict result without publishing it', () => {
    const api = setup()
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'editor' },
    })
    const initial = api.handle({
      method: 'POST',
      path: path + '/working-copy',
    }) as DomainWorkingCopy
    const base = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: { etag: initial.etag, message: 'base', requestId: 'base' },
    }) as DomainCommitOutcome
    api.state.user = { ...demoUser }
    const other = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    api.state.user = { ...adminUser }
    const mine = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/statement-zh/text',
      body: { etag: base.copy.etag, text: 'My introduction' },
    }) as DomainWorkingCopy
    api.state.user = { ...demoUser }
    const theirs = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/statement-zh/text',
      body: { etag: other.etag, text: 'Remote example' },
    }) as DomainWorkingCopy
    api.handle({
      method: 'POST',
      path: path + '/commits',
      body: { etag: theirs.etag, message: 'remote', requestId: 'remote' },
    })
    api.state.user = { ...adminUser }
    const outcome = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: { etag: mine.etag, message: 'local', requestId: 'local' },
    }) as DomainCommitOutcome
    const merge = outcome.merge!
    expect(merge.result.conflicts).toHaveLength(1)
    const content = 'My introduction\n\nRemote example'
    const ref = api.handle({
      method: 'POST',
      path: path + '/blobs',
      body: { bytes: [...new TextEncoder().encode(content)] },
    }) as DomainBlobRef
    const tree = structuredClone(merge.result.tree)
    tree.entries.find((entry) => entry.id === 'statement-zh')!.blob = ref
    const resolved = api.handle({
      method: 'PUT',
      path: path + '/merges/' + merge.id,
      body: { etag: merge.etag, tree, resolved: [{ entryId: 'statement-zh', field: 'entry' }] },
    }) as DomainMergeSession
    const completed = api.handle({
      method: 'POST',
      path: path + '/merges/' + merge.id + '/complete',
      body: { etag: resolved.etag },
    }) as DomainWorkingCopy
    expect(completed.mergeId).toBeUndefined()
    expect(completed.tree.entries.find((entry) => entry.id === 'statement-zh')?.blob).toEqual(ref)
    expect(api.state.problems[0].statementMd).not.toContain('My introduction')
    const final = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: { etag: completed.etag, message: 'combined', requestId: 'combined' },
    }) as DomainCommitOutcome
    expect(final.commit?.revision).toBe(3)
  })
  it('updates the saved test order together with adding and deleting a test', () => {
    const api = setup()
    let copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    copy = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/case1',
      body: {
        etag: copy.etag,
        entry: {
          id: 'case1',
          kind: 'test',
          path: 'vertex/tests/case1.json',
          attributes: { format: 'json' },
        },
        text: JSON.stringify(defaultTest()),
      },
    }) as DomainWorkingCopy
    let metadata = api.handle({
      method: 'GET',
      path: path + '/materials/problem',
    }) as DomainMaterialView
    expect(metadata.metadata?.testOrder).toEqual(['case1'])
    api.handle({
      method: 'DELETE',
      path: path + '/working-copy/entries/case1',
      body: { etag: copy.etag },
    })
    metadata = api.handle({
      method: 'GET',
      path: path + '/materials/problem',
    }) as DomainMaterialView
    expect(metadata.metadata?.testOrder).toEqual([])
  })
  it('initializes typed material, saves without revisions, and commits explicitly', () => {
    const api = setup()
    let copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    const metadata = api.handle({
      method: 'GET',
      path: path + '/materials/problem',
    }) as DomainMaterialView
    expect(metadata.metadata?.title).toBe('两数之和')
    expect(metadata.metadata?.judgeType).toBe('normal')
    expect(
      (api.handle({ method: 'GET', path: path + '/changes' }) as DomainContentComparison).changes,
    ).toHaveLength(2)
    const token = copy.etag
    copy = api.handle({
      method: 'PUT',
      path: path + '/working-copy',
      body: { etag: copy.etag, tree: copy.tree },
    }) as DomainWorkingCopy
    expect(copy.etag).toBe(token)
    copy = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/statement-zh/text',
      body: { etag: copy.etag, text: 'Changed draft' },
    }) as DomainWorkingCopy
    expect(api.handle({ method: 'GET', path: path + '/commits' })).toEqual({ items: [] })
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/working-copy',
        body: { etag: token, tree: copy.tree },
      }),
    ).toThrow('已变化')
    const request = { etag: copy.etag, message: 'Revise statement', requestId: 'first' }
    const result = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: request,
    }) as DomainCommitOutcome
    expect(result.commit?.revision).toBe(1)
    const retried = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: request,
    }) as DomainCommitOutcome
    expect(retried.commit?.revision).toBe(1)
    expect(
      (api.handle({ method: 'GET', path: path + '/changes' }) as DomainContentComparison).changes,
    ).toEqual([])
    expect(api.state.problems[0].statementMd).not.toBe('Changed draft')
  })
  it('keeps collaborator drafts and uploaded blobs private until committed', () => {
    const api = setup()
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'editor' },
    })
    const ownerCopy = api.handle({
      method: 'POST',
      path: path + '/working-copy',
    }) as DomainWorkingCopy
    const blob = api.handle({
      method: 'POST',
      path: path + '/blobs',
      body: { bytes: [1, 2, 3] },
    }) as DomainBlobRef
    api.state.user = { ...demoUser }
    expect(() => api.handle({ method: 'GET', path: path + '/working-copy' })).toThrow('尚未创建')
    expect(() => api.handle({ method: 'GET', path: path + '/blobs/' + blob.sha256 })).toThrow(
      '不可访问',
    )
    const editorCopy = api.handle({
      method: 'POST',
      path: path + '/working-copy',
    }) as DomainWorkingCopy
    expect(editorCopy.etag).not.toBe(ownerCopy.etag)
    editorCopy.tree.entries.push({
      id: 'stolen',
      path: 'secret.cpp',
      kind: 'source',
      blob,
      attributes: {},
    })
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/working-copy',
        body: { etag: editorCopy.etag, tree: editorCopy.tree },
      }),
    ).toThrow('不可访问')
  })
})
