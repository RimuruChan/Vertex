import { describe, expect, it } from 'vitest'
import type { DomainTreeEntry } from '@/generated/api/model'
import { findMaterials, materialDestination } from './material-search'

const entry = (id: string, kind: string, path: string, label = ''): DomainTreeEntry => ({
  id,
  kind,
  path,
  attributes: { label },
  blob: { sha256: '', bytes: 0 },
})
describe('material finder', () => {
  it('finds programs by their saved display name as well as their path', () => {
    const entries = [entry('accepted', 'program', 'vertex/programs/accepted.json')]
    expect(
      findMaterials(entries, '1', '正确参考解', '程序', { accepted: '正确参考解' })[0].name,
    ).toBe('正确参考解')
    expect(
      findMaterials(entries, '1', 'accepted.json', '程序', { accepted: '正确参考解' }),
    ).toHaveLength(1)
  })
  it('searches complete paths with multiple words and prioritizes exact names', () => {
    const entries = [
      entry('a', 'source', 'solutions/main.cpp', 'main.cpp helper'),
      entry('b', 'source', 'solutions/main.cpp', 'main.cpp'),
    ]
    expect(findMaterials(entries, '1', ' main.cpp ', '全部').map((item) => item.entry.id)).toEqual([
      'b',
      'a',
    ])
    expect(findMaterials(entries, '1', 'SOLUTIONS MAIN', '程序')).toHaveLength(2)
    expect(findMaterials(entries, '1', 'main', '题面')).toHaveLength(0)
  })
  it('opens the right test partition and escapes identifiers', () => {
    const target = materialDestination('1', entry('plan&x=1', 'generation', 'plan.json'))!
    const url = new URL(target.to, 'https://vertex.test')
    expect(url.searchParams.get('entry')).toBe('plan&x=1')
    expect(url.searchParams.get('plan')).toBe('plan&x=1')
    expect(url.searchParams.get('data')).toBe('generation')
    expect(url.searchParams.has('x')).toBe(false)
  })
  it('includes private resources without crossing to public problem URLs', () => {
    const result = findMaterials(
      [entry('private', 'resource', 'resources/reference.md', '解题记录')],
      '1',
      '解题',
      '附件',
    )
    expect(result[0].to).toBe('/authoring/1/assets?entry=private')
    expect(findMaterials([entry('x', 'unknown', 'x')], '1', '', '全部')).toEqual([])
  })
})
