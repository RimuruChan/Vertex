import { describe, expect, it } from 'vitest'
import { reviewTestFiles, missingTestMaterials } from './data-review'
import { defaultTest } from '@/lib/authoring-materials'
import type { DomainMaterialView, DomainTreeEntry } from '@/generated/api/model'

describe('test data review', () => {
  it('keeps valid pairs when an unsupported file is selected', () => {
    const result = reviewTestFiles([
      new File([''], '2.in'),
      new File([''], '2.out'),
      new File([''], 'notes.txt'),
    ])
    expect(result.readyCount).toBe(1)
    expect(result.rejected.map(({ index }) => index)).toEqual([2])
    expect(result.pairs[0].files.map(({ index }) => index)).toEqual([0, 1])
  })
  it('retains duplicate files individually so removing one repairs its pair', () => {
    const files = [
      new File([''], 'case.IN'),
      new File(['a'], 'CASE.ans'),
      new File(['b'], 'case.out'),
    ]
    const result = reviewTestFiles(files)
    expect(result.pairs).toHaveLength(1)
    expect(result.pairs[0].files).toHaveLength(3)
    expect(result.readyCount).toBe(0)
    expect(reviewTestFiles(files.filter((_, index) => index !== 2)).readyCount).toBe(1)
  })
  it('allows an incomplete pair to be repaired without changing the other pair', () => {
    const files = [new File([''], '10.in'), new File([''], '2.in'), new File([''], '2.out')]
    expect(reviewTestFiles(files).pairs.map(({ name }) => name)).toEqual(['2', '10'])
    expect(reviewTestFiles(files).readyCount).toBe(1)
    expect(reviewTestFiles([...files, new File([''], '10.ans')]).readyCount).toBe(2)
  })
  it('checks referenced materials without treating generated tests as missing files', () => {
    const entry: DomainTreeEntry = {
      id: 'case',
      kind: 'test',
      path: 'case.json',
      attributes: {},
      blob: { sha256: '', bytes: 0 },
    }
    const item: DomainMaterialView = {
      entry,
      test: {
        ...defaultTest(),
        input: { kind: 'generator', generator: 'generator', entry: '', arguments: [] },
        answer: { kind: 'solution', solution: 'solution', entry: '' },
      },
    }
    expect(
      missingTestMaterials(item, [
        { ...entry, id: 'generator' },
        { ...entry, id: 'solution' },
      ]),
    ).toEqual([])
    expect(missingTestMaterials(item, [{ ...entry, id: 'generator' }])).toEqual(['标准解'])
    expect(missingTestMaterials({ entry, error: 'invalid document' }, [])).toEqual([])
  })
})
