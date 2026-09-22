import { describe, expect, it } from 'vitest'
import { checkMaterialTarget } from './check-material-target'

describe('check material repair links', () => {
  it.each([
    ['program', 'programs'],
    ['source', 'programs'],
    ['statement', 'statement'],
    ['metadata', 'overview'],
  ])('opens the %s editor with the exact entry identity', (kind, section) => {
    const id = '材料/source + main.cpp?mode=1&entry=other#tail'
    const target = new URL(checkMaterialTarget('1001', { kind, id })!, 'https://vertex.test')
    expect(target.pathname).toBe(`/authoring/1001/${section}`)
    expect([...target.searchParams]).toEqual([['entry', id]])
    expect(target.hash).toBe('')
  })

  it.each(['test', 'input', 'answer'])(
    'routes the %s material to tests without adding accidental query parameters',
    (kind) => {
      const id = 'sample/输入 1&data=group'
      const target = new URL(checkMaterialTarget('1001', { kind, id })!, 'https://vertex.test')
      expect(target.pathname).toBe('/authoring/1001/tests')
      expect([...target.searchParams]).toEqual([['entry', id]])
    },
  )

  it.each(['group', 'validation'])('selects both the %s tab and the requested material', (kind) => {
    const id = `${kind}/边界?data=generation&plan=wrong`
    const target = new URL(checkMaterialTarget('1001', { kind, id })!, 'https://vertex.test')
    expect(target.pathname).toBe('/authoring/1001/tests')
    expect([...target.searchParams]).toEqual([
      ['entry', id],
      ['data', kind],
    ])
  })

  it('keeps the generation plan and entry IDs identical after URL decoding', () => {
    const id = '方案 / #1?plan=other&data=test+'
    const target = new URL(
      checkMaterialTarget('1001', { kind: 'generation', id })!,
      'https://vertex.test',
    )
    expect(target.pathname).toBe('/authoring/1001/tests')
    expect([...target.searchParams]).toEqual([
      ['entry', id],
      ['data', 'generation'],
      ['plan', id],
    ])
    expect(target.hash).toBe('')
  })

  it.each(['asset', 'resource', 'unknown', 'generation?data=test'])(
    'does not create a misleading repair link for unsupported kind %s',
    (kind) => {
      expect(checkMaterialTarget('1001', { kind, id: 'existing-entry' })).toBeUndefined()
    },
  )
})
