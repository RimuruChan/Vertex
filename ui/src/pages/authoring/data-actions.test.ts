import { describe, expect, it, vi } from 'vitest'
import { pairTestFiles, pairDraft, saveTestDrafts } from './data-actions'
import { defaultMetadata } from '@/lib/authoring-materials'
import type { DomainWorkingCopy } from '@/generated/api/model'
import type { useDomainAPI } from '@/domain/useDomainAPI'

describe('test-data authoring interactions', () => {
  it('pairs input and answers by stem and sorts numeric names', () => {
    const pairs = pairTestFiles([
      new File(['10'], 'case10.in'),
      new File(['2'], 'case2.IN'),
      new File(['a'], 'case2.ans'),
      new File(['a'], 'case10.out'),
    ])
    expect(pairs.map((pair) => pair.name)).toEqual(['case2', 'case10'])
    expect(pairs.every((pair) => pair.input && pair.answer && !pair.error)).toBe(true)
    const ambiguous = pairTestFiles([
      new File([''], '1.in'),
      new File([''], '1.out'),
      new File([''], '1.ans'),
    ])
    expect(() => pairDraft(ambiguous[0], false)).toThrow('重复')
    expect(() => pairDraft(pairTestFiles([new File([''], '1.in')])[0], false)).toThrow('不完整')
    expect(() => pairTestFiles([new File([''], 'notes.txt')])).toThrow('数据文件')
  })
  function fixture() {
    const entry = {
      id: 'problem',
      kind: 'metadata',
      path: 'vertex/problem.json',
      attributes: {},
      blob: { sha256: 'a'.repeat(64), bytes: 10 },
    }
    const copy: DomainWorkingCopy = {
      etag: 'unchanged-token',
      updatedAt: '',
      tree: { entries: [entry] },
    }
    const blobs: File[] = []
    const upload = vi.fn(async (_id: string, { file }: { file: File }) => {
      blobs.push(file)
      return { sha256: blobs.length.toString(16).padStart(64, '0'), bytes: file.size }
    })
    const save = vi.fn(async (_id: string, body: { tree: DomainWorkingCopy['tree'] }) => ({
      ...copy,
      etag: 'next-token',
      tree: body.tree,
    }))
    const api = {
      postApiAuthoringProblemsIdBlobs: upload,
      putApiAuthoringProblemsIdWorkingCopy: save,
      getApiAuthoringProblemsIdMaterialsEntryId: vi.fn(async () => ({
        entry,
        metadata: defaultMetadata('Sum'),
      })),
    } as unknown as ReturnType<typeof useDomainAPI>
    return { copy, blobs, upload, save, api }
  }
  it('creates both files, descriptor and order with one atomic copy save', async () => {
    const { copy, api, save, blobs } = fixture(),
      before = structuredClone(copy)
    const input = new File([new Uint8Array([0, 255, 13, 10])], '1.in'),
      answer = new File(['3\n'], '1.ans')
    const next = await saveTestDrafts(api, '1000', copy, [
      pairDraft({ name: 'binary', input, answer }, true),
    ])
    expect(save).toHaveBeenCalledTimes(1)
    expect(save.mock.calls[0][1]).toMatchObject({ etag: 'unchanged-token' })
    expect(copy).toEqual(before)
    expect(new Uint8Array(await blobs[0].arrayBuffer())).toEqual(new Uint8Array([0, 255, 13, 10]))
    const test = JSON.parse(await blobs.find((file) => file.name === 'test.json')!.text())
    const metadata = JSON.parse(await blobs.find((file) => file.name === 'problem.json')!.text())
    expect(test).toMatchObject({
      name: 'binary',
      isSample: true,
      input: { kind: 'file' },
      answer: { kind: 'file' },
    })
    expect(metadata.testOrder).toEqual(
      next.tree.entries.filter((entry) => entry.kind === 'test').map((entry) => entry.id),
    )
  })
  it('does not save a partial test when upload fails', async () => {
    const { copy, api, upload, save } = fixture()
    upload.mockRejectedValueOnce(new Error('upload failed'))
    await expect(
      saveTestDrafts(api, '1000', copy, [
        pairDraft(
          { name: 'one', input: new File(['1'], '1.in'), answer: new File(['1'], '1.out') },
          false,
        ),
      ]),
    ).rejects.toThrow('upload failed')
    expect(save).not.toHaveBeenCalled()
    expect(copy.etag).toBe('unchanged-token')
  })
})
