import { expect, it, vi } from 'vitest'
import type { DomainWorkingCopy } from '@/generated/api/model'
import type { useDomainAPI } from '@/domain/useDomainAPI'
import { newProgram, saveProgramDraft } from './program-draft'

function fixture() {
  const copy: DomainWorkingCopy = { etag: 'expected-copy', updatedAt: '', tree: { entries: [] } }
  const bodies: Blob[] = []
  const upload = vi.fn(async (_id: string, { file }: { file: Blob }) => {
    bodies.push(file)
    return { sha256: String(bodies.length).padStart(64, '0'), bytes: file.size }
  })
  const write = vi.fn(
    async (_id: string, body: { etag: string; tree: DomainWorkingCopy['tree'] }) => ({
      ...copy,
      tree: body.tree,
      etag: 'next-copy',
    }),
  )
  const api = {
    postApiAuthoringProblemsIdBlobs: upload,
    putApiAuthoringProblemsIdWorkingCopy: write,
  } as unknown as ReturnType<typeof useDomainAPI>
  return { api, copy, bodies, upload, write }
}
it('creates a complete program and main code in one copy transaction', async () => {
  const f = fixture(),
    draft = newProgram('标准解', 'solution', 'python')
  const result = await saveProgramDraft(f.api, 'p', f.copy, draft, [])
  expect(f.write).toHaveBeenCalledTimes(1)
  expect(f.write.mock.calls[0][1].etag).toBe('expected-copy')
  expect(result.copy.tree.entries.map((e) => e.kind).sort()).toEqual(['program', 'source'])
  const definition = JSON.parse(await f.bodies.at(-1)!.text())
  expect(definition.files).toEqual([definition.entryPoint])
  expect(result.sources[0].attributes.language).toBe('python')
  expect(await f.bodies[0].text()).toContain('def main():')
})
it('isolates shared code while retaining a multi-file program layout and old references', async () => {
  const f = fixture(),
    draft = newProgram('解法一', 'solution', 'cpp')
  const main = draft.sources[0]
  main.path = 'imported/src/main.cpp'
  main.blob = { sha256: 'a'.repeat(64), bytes: 12 }
  const header = {
    ...main,
    id: 'header',
    path: 'imported/include/helper.h',
    blob: { sha256: 'b'.repeat(64), bytes: 9 },
  }
  draft.sources.push(header)
  draft.program.directory = 'imported'
  draft.program.files.push('header')
  f.copy.tree.entries = [draft.entry, main, header]
  const other = { ...draft.program, name: '解法二' }
  const result = await saveProgramDraft(f.api, 'p', f.copy, draft, [other])
  expect(result.program.entryPoint).not.toBe(main.id)
  expect(result.sources.map((e) => e.path.slice(result.program.directory.length + 1))).toEqual([
    'src/main.cpp',
    'include/helper.h',
  ])
  expect(result.copy.tree.entries.find((e) => e.id === main.id)?.blob.sha256).toBe('a'.repeat(64))
  expect(result.copy.tree.entries.find((e) => e.id === 'header')?.blob.sha256).toBe('b'.repeat(64))
  expect(other.files).toContain(main.id)
  expect(f.write).toHaveBeenCalledTimes(1)
})
it('does not publish a partially uploaded program', async () => {
  const f = fixture(),
    draft = newProgram('解法', 'solution', 'cpp')
  f.upload.mockRejectedValueOnce(new Error('offline'))
  await expect(saveProgramDraft(f.api, 'p', f.copy, draft, [])).rejects.toThrow('offline')
  expect(f.write).not.toHaveBeenCalled()
})
it('rejects stale program content even if the parent supplies a newer copy token', async () => {
  const f = fixture(),
    draft = newProgram('标准解', 'solution', 'cpp')
  draft.entry.blob = { sha256: 'a'.repeat(64), bytes: 12 }
  f.copy.tree.entries = [{ ...draft.entry, blob: { sha256: 'b'.repeat(64), bytes: 12 } }]
  await expect(saveProgramDraft(f.api, 'p', f.copy, draft, [])).rejects.toThrow('另一页面')
  expect(f.write).not.toHaveBeenCalled()
  expect(f.upload).not.toHaveBeenCalled()
})
it('preserves include filenames when old unassigned code occupies the same location', async () => {
  const f = fixture(),
    draft = newProgram('标准解', 'solution', 'cpp')
  const main = draft.sources[0]
  main.blob = { sha256: 'a'.repeat(64), bytes: 20 }
  const old = { ...main, id: 'old-helper', path: `${draft.program.directory}/helper.h` }
  f.copy.tree.entries = [draft.entry, main, old]
  const added = { ...old, id: 'new-helper', blob: { sha256: '', bytes: 0 } }
  draft.sources.push(added)
  draft.program.files.push(added.id)
  draft.changes = { [added.id]: new Blob(['int helper();']) }
  const result = await saveProgramDraft(f.api, 'p', f.copy, draft, [])
  expect(result.sources.map((e) => e.path.slice(result.program.directory.length + 1))).toEqual([
    'main.cpp',
    'helper.h',
  ])
  expect(result.copy.tree.entries.find((e) => e.id === old.id)).toEqual(old)
  expect(result.program.directory).not.toBe(draft.program.directory)
})
it('does not rewrite imported root-relative layouts when only settings change', async () => {
  const f = fixture(),
    draft = newProgram('解法', 'solution', 'cpp')
  draft.program.directory = ''
  draft.changes = {}
  draft.sources[0].blob = { sha256: 'a'.repeat(64), bytes: 4 }
  f.copy.tree.entries = [draft.entry, ...draft.sources]
  const result = await saveProgramDraft(f.api, 'p', f.copy, draft, [])
  expect(result.program.directory).toBe('')
  expect(result.sources[0].path).toBe(draft.sources[0].path)
  expect(f.upload).toHaveBeenCalledTimes(1)
})
