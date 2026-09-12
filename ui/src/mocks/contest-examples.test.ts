import { describe, expect, it } from 'vitest'
import type { DtoRankboardResponse } from '@/generated/api/model'
import { createFixtures, mockID, type MockState } from './fixtures'
import { ensureContestExamples } from './contest-examples'
import { createMockAPI } from './api'
import { adminUser } from './identities'

const now = Date.parse('2030-01-01T12:00:00Z')

describe('contest scoreboard examples', () => {
  it('adds five representative scenarios once and preserves existing data and edits', () => {
    const state: MockState = createFixtures(now)
    state.contests[0].title = '保留手工修改'
    const contests = structuredClone(state.contests)
    const submissions = structuredClone(state.submissions)
    expect(ensureContestExamples(state, now)).toBe(true)
    expect(state.contests.slice(0, contests.length)).toEqual(contests)
    expect(state.submissions.slice(0, submissions.length)).toEqual(submissions)
    expect(state.contests.slice(contests.length).map((c) => [c.format, !!c.freezeAt])).toEqual([
      ['icpc', true],
      ['oi', false],
      ['ioi', false],
      ['leduo', false],
      ['cf', false],
    ])
    expect(state.contests).toHaveLength(8)
    expect(state.contests.slice(3).map((c) => c.publicId)).toEqual(['5', '7', '8', '10', '12'])
    const once = JSON.stringify(state)
    expect(ensureContestExamples(state, now + 86400000)).toBe(false)
    expect(JSON.stringify(state)).toBe(once)
    expect(new Set(state.submissions.map((s) => s.id)).size).toBe(state.submissions.length)
  })

  it('removes unused generated duplicates from saved demos without orphaning their data', () => {
    const state: MockState = createFixtures(now)
    ensureContestExamples(state, now)
    const retained = state.contests.find((c) => c.id === mockID(9300, 2))!
    const id = mockID(9300, 1)
    state.contests.push({
      ...retained,
      id,
      publicId: '4',
      title: 'ICPC 榜单演示 · 未封榜',
      freezeAt: undefined,
      medals: undefined,
    })
    const source = state.submissions.find((s) => s.contestId === retained.id)!
    const submission = { ...source, id: mockID(9400, 9000), contestId: id, contestPublicId: '4' }
    state.submissions.push(submission)
    state.registrations[source.userId].push(id)
    state.contestProblemIds[id] = [source.problemId]
    state.contestProblemVersions[id] = { [source.problemId]: 1 }
    state.contestEntries![id] = []
    state.staff[id] = []
    state.pending[submission.id] = { started: now, verdict: 'Accepted' }
    state.submissionGenerations[submission.id] = 1
    state.scoreboardExamplesVersion = 2
    expect(ensureContestExamples(state, now)).toBe(true)
    expect(state.contests).toHaveLength(8)
    expect(state.contests.find((c) => c.id === retained.id)).toEqual(retained)
    expect(state.submissions.some((s) => s.contestId === id)).toBe(false)
    expect(state.registrations[source.userId]).not.toContain(id)
    expect(state.contestProblemIds[id]).toBeUndefined()
    expect(state.contestProblemVersions[id]).toBeUndefined()
    expect(state.contestEntries![id]).toBeUndefined()
    expect(state.staff[id]).toBeUndefined()
    expect(state.pending[submission.id]).toBeUndefined()
    expect(state.submissionGenerations[submission.id]).toBeUndefined()
  })

  it.each(['rename', 'submission', 'settings'] as const)(
    'preserves an example with user %s changes',
    (kind) => {
      const state: MockState = createFixtures(now)
      ensureContestExamples(state, now)
      const retained = state.contests.find((c) => c.id === mockID(9300, 2))!
      const duplicate = {
        ...retained,
        id: mockID(9300, 1),
        publicId: '4',
        title: kind === 'rename' ? '自定义练习赛' : 'ICPC 榜单演示 · 未封榜',
        medals: kind === 'settings' ? retained.medals : undefined,
      }
      state.contests.push(duplicate)
      if (kind === 'submission')
        state.submissions.push({
          ...state.submissions[0],
          id: 'manual-submission',
          contestId: duplicate.id,
        })
      state.scoreboardExamplesVersion = 2
      ensureContestExamples(state, now)
      expect(state.contests).toContainEqual(duplicate)
    },
  )

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
