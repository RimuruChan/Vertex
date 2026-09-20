import { expect, it } from 'vitest'
import { materialChanges, materialValue } from './material-diff'

it('describes settings changes as named fields rather than JSON line noise', () => {
  const changes = materialChanges(
    '{"schemaVersion":1,"title":"Sum","comparison":{"kind":"tokens"}}',
    '{"schemaVersion":1,"title":"Sum","comparison":{"kind":"exact"}}',
  )
  expect(changes).toEqual([
    { key: 'comparison.kind', label: '答案比较', before: 'tokens', after: 'exact' },
  ])
  expect(materialValue('memoryLimitKb', 262144, {})).toBe('256 MiB')
  expect(materialValue('mainSolution', 'source-id', { 'source-id': '标准参考解' })).toBe(
    '标准参考解',
  )
})
it('does not obscure a test order change behind identical counts', () => {
  const labels = { a: '小数据', b: '边界数据' }
  expect(materialValue('testOrder', ['a', 'b'], labels)).toBe('小数据 → 边界数据')
  expect(materialValue('testOrder', ['b', 'a'], labels)).toBe('边界数据 → 小数据')
})
