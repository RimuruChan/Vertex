import { expect, it, vi } from 'vitest'
import type { DomainProgramSaveInput, DomainWorkingCopy } from '@/generated/api/model'
import type { useDomainAPI } from '@/domain/useDomainAPI'
import { newProgram, saveProgramDraft } from './program-draft'

function fixture() {
  const copy: DomainWorkingCopy = { etag: 'expected-copy', updatedAt: '', tree: { entries: [] } }
  const upload = vi.fn(async (_id: string, { file }: { file: Blob }) => ({
    sha256: 'a'.repeat(64),
    bytes: file.size,
  }))
  const write = vi.fn(async (_id: string, body: DomainProgramSaveInput) => ({
    copy,
    program: body.program,
    sources: body.sources.map((s) => ({
      id: s.id,
      kind: 'source',
      path: 'server-owned/' + s.relativeName,
      attributes: { label: s.name },
      blob: s.blob,
    })),
    remap: {},
  }))
  const rawSave = vi.fn()
  const api = {
    postApiAuthoringProblemsIdBlobs: upload,
    putApiAuthoringProblemsIdPrograms: write,
    putApiAuthoringProblemsIdWorkingCopy: rawSave,
  } as unknown as ReturnType<typeof useDomainAPI>
  return { api, copy, upload, write, rawSave }
}
it('uploads code then sends one typed program operation instead of replacing the file tree', async () => {
  const f = fixture(),
    draft = newProgram('标准解', 'solution', 'python')
  const result = await saveProgramDraft(f.api, 'p', f.copy, draft, [])
  expect(f.write).toHaveBeenCalledTimes(1)
  expect(f.write.mock.calls[0][1]).toMatchObject({
    etag: 'expected-copy',
    id: draft.entry.id,
    baseHash: '',
    program: { name: '标准解', role: 'solution' },
  })
  expect(f.write.mock.calls[0][1].sources[0].relativeName).toBe('main.py')
  expect(result.sources[0].path).toBe('server-owned/main.py')
  expect(f.rawSave).not.toHaveBeenCalled()
})
it('preserves imported relative layout and version preconditions in the request', async () => {
  const f = fixture(),
    draft = newProgram('解法', 'solution', 'cpp'),
    source = draft.sources[0]
  source.path = 'imported/src/main.cpp'
  source.blob = { sha256: 'b'.repeat(64), bytes: 12 }
  draft.program.directory = 'imported'
  f.copy.tree.entries = [draft.entry, source]
  await saveProgramDraft(f.api, 'p', f.copy, draft, [])
  expect(f.write.mock.calls[0][1].sources[0]).toMatchObject({
    relativeName: 'src/main.cpp',
    baseHash: 'b'.repeat(64),
  })
})
it('never applies a partial program when uploading fails', async () => {
  const f = fixture()
  f.upload.mockRejectedValueOnce(new Error('offline'))
  await expect(
    saveProgramDraft(f.api, 'p', f.copy, newProgram('解法', 'solution', 'cpp'), []),
  ).rejects.toThrow('offline')
  expect(f.write).not.toHaveBeenCalled()
  expect(f.rawSave).not.toHaveBeenCalled()
})
it('keeps a stale local draft from being sent with a refreshed copy token', async () => {
  const f = fixture(),
    draft = newProgram('解法', 'solution', 'cpp')
  draft.entry.blob = { sha256: 'a'.repeat(64), bytes: 12 }
  f.copy.tree.entries = [{ ...draft.entry, blob: { sha256: 'b'.repeat(64), bytes: 12 } }]
  await expect(saveProgramDraft(f.api, 'p', f.copy, draft, [])).rejects.toThrow('另一页面')
  expect(f.write).not.toHaveBeenCalled()
})
