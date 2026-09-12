import { describe, expect, it } from 'vitest'
import { assignMedals, medalSettings } from './contest-medals'

describe('contest medals', () => {
  it('uses only solved entrants and gives ties the higher medal at a boundary', () => {
    const rows = [
      { rank: 1, solved: 0 },
      { rank: 2, solved: 1 },
      { rank: 2, solved: 2 },
      { rank: 4, solved: 1 },
      { rank: 5, solved: 1 },
      { rank: 6, solved: 0 },
      { rank: 7, solved: 1 },
    ]
    expect(assignMedals(rows, { mode: 'count', gold: 1, silver: 2, bronze: 1 })).toEqual({
      mode: 'count',
      eligible: 5,
      gold: 2,
      silver: 1,
      bronze: 1,
    })
    expect(rows).toMatchObject([
      { solved: 0 },
      { medal: 'gold' },
      { medal: 'gold' },
      { medal: 'silver' },
      { medal: 'bronze' },
      { solved: 0 },
      { solved: 1 },
    ])
    expect(assignMedals(rows, medalSettings(undefined))).toBeUndefined()
    expect(rows.every((row) => !('medal' in row))).toBe(true)
  })
  it('rounds each quota down using eligible entrants, including zero quotas', () => {
    const rows = Array.from({ length: 10 }, (_, i) => ({ rank: i + 1, solved: i < 7 ? 1 : 0 }))
    expect(assignMedals(rows, { mode: 'percentage', gold: 10, silver: 30, bronze: 40 })).toEqual({
      mode: 'percentage',
      eligible: 7,
      gold: 0,
      silver: 2,
      bronze: 2,
    })
    expect(
      assignMedals([{ rank: 1, solved: 0 }], { mode: 'count', gold: 10, silver: 0, bronze: 0 })
        ?.gold,
    ).toBe(0)
    expect(
      assignMedals(
        [
          { rank: 1, solved: 1 },
          { rank: 1, solved: 1 },
          { rank: 3, solved: 1 },
        ],
        { mode: 'count', gold: 0, silver: 0, bronze: 1 },
      )?.bronze,
    ).toBe(2)
  })
  it('rejects invalid values without accepting string or fractional quotas', () => {
    for (const value of [
      { mode: 'bad' },
      { mode: 'count', gold: -1 },
      { mode: 'count', bronze: 100001 },
      { mode: 'percentage', gold: 51, silver: 50 },
      { mode: 'count', gold: 1.5 },
      { mode: 'count', gold: '2' },
    ])
      expect(() => medalSettings(value)).toThrow()
    expect(medalSettings({ mode: 'percentage', gold: 20, silver: 30, bronze: 50 }).gold).toBe(20)
  })
})
