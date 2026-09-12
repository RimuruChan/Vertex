import { describe, expect, it } from 'vitest'
import type { DtoRankboardResponse } from '@/generated/api/model'
import { createFixtures, type MockState } from './fixtures'
import { ensureContestExamples } from './contest-examples'
import { createMockAPI } from './api'
import { adminUser } from './identities'

const now = Date.parse('2030-01-01T12:00:00Z')

describe('contest scoreboard examples', () => {
  it('adds all six scenarios once and preserves existing data and edits', () => {
    const state: MockState = createFixtures(now)
    state.contests[0].title = '保留手工修改'
    const contests = structuredClone(state.contests)
    const submissions = structuredClone(state.submissions)
    expect(ensureContestExamples(state, now)).toBe(true)
    expect(state.contests.slice(0, contests.length)).toEqual(contests)
    expect(state.submissions.slice(0, submissions.length)).toEqual(submissions)
    expect(state.contests.slice(contests.length).map((c) => [c.format, !!c.freezeAt])).toEqual([
      ['icpc', false],
      ['icpc', true],
      ['oi', false],
      ['oi', false],
      ['ioi', false],
      ['ioi', true],
      ['leduo', false],
      ['leduo', true],
      ['cf', false],
      ['cf', true],
    ])
    const once = JSON.stringify(state)
    expect(ensureContestExamples(state, now + 86400000)).toBe(false)
    expect(JSON.stringify(state)).toBe(once)
    expect(new Set(state.submissions.map((s) => s.id)).size).toBe(state.submissions.length)
  })

  it('derives populated standings and masks only frozen public results', () => {
    const state: MockState = createFixtures(now)
    ensureContestExamples(state, now)
    state.user = { ...adminUser }
    const api = createMockAPI(state, () => now)
    for (const contest of state.contests.slice(3)) {
      const board = (jury = false) =>
        api.handle({
          method: 'GET',
          path: `/api/contests/${contest.id}/rankboard`,
          params: jury ? { view: 'jury' } : {},
        }) as DtoRankboardResponse
      const internal = board(true)
      if (contest.format === 'oi' && now <= Date.parse(contest.endAt)) {
        expect(() => board()).toThrow('不反馈')
        expect(internal.rows).toHaveLength(26)
        continue
      }
      const publicBoard = board()
      expect(publicBoard.rows).toHaveLength(26)
      expect(internal.problems).toHaveLength(8)
      expect(internal.rows.some((row) => row.solved > 0)).toBe(true)
      expect(internal.rows.some((row) => row.cells.every((cell) => cell.attempts === 0))).toBe(true)
      expect(internal.rows.some((row) => row.cells.some((cell) => cell.firstSolver))).toBe(true)
      expect(internal.frozen).toBe(false)
      expect(publicBoard.frozen).toBe(!!contest.freezeAt)
      expect(publicBoard.rows.some((row) => row.hasPending)).toBe(!!contest.freezeAt)
      if (['oi', 'ioi', 'leduo'].includes(contest.format))
        expect(
          internal.rows.some((row) => row.cells.some((cell) => cell.score > 0 && !cell.solvedAt)),
        ).toBe(true)
    }
  })
})
