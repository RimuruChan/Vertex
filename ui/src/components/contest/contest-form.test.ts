import { describe, expect, it } from 'vitest'
import { createFixtures } from '@/mocks/fixtures'
import {
  contestDraft,
  contestPayload,
  compositionPayload,
  nextProblemLabel,
  newContestDraft,
  canPrepareContest,
} from './contest-form'
import type { DtoContestProblemResponse } from '@/generated/api/model'

describe('contest detail forms', () => {
  it('creates a private draft and serializes only editable fields', () => {
    const draft = newContestDraft(Date.parse('2030-01-01T00:00:00Z'))
    draft.title = ' New round '
    expect(contestPayload(draft)).toMatchObject({
      title: 'New round',
      visibility: 'private',
      beginAt: '2030-01-02T00:00:00.000Z',
      endAt: '2030-01-02T05:00:00.000Z',
    })
    const source = createFixtures().contests[0]
    const value = contestPayload(contestDraft(source), source.visibility)
    expect(value).not.toHaveProperty('ownerId')
    expect(value).not.toHaveProperty('permissions')
    expect(value).not.toHaveProperty('id')
  })
  it('validates timing and keeps a password unchanged only for an existing password contest', () => {
    const draft = { ...newContestDraft(), title: 'Round' }
    expect(() => contestPayload({ ...draft, endAt: draft.beginAt })).toThrow('起止')
    expect(() => contestPayload({ ...draft, freezeAt: draft.endAt })).toThrow('封榜')
    expect(() => contestPayload({ ...draft, unfreezeAt: draft.endAt })).toThrow('解榜')
    expect(() => contestPayload({ ...draft, visibility: 'password' })).toThrow('密码')
    expect(
      contestPayload({ ...draft, visibility: 'password' }, 'password').password,
    ).toBeUndefined()
  })
  it('preserves label, score and colour on reorder and never sends a new version implicitly', () => {
    const entry = (id: string, label: string, points: number) =>
      ({ problemId: id, label, color: '#64748b', points, version: 7 }) as DtoContestProblemResponse
    const a = entry('a', 'Warmup', 50),
      b = entry('b', 'B2', 300)
    expect(compositionPayload([b, a])).toEqual({
      problems: [
        { problemId: 'b', label: 'B2', color: '#64748b', points: 300 },
        { problemId: 'a', label: 'Warmup', color: '#64748b', points: 50 },
      ],
    })
    expect(() => compositionPayload([a, { ...b, label: 'Warmup' }])).toThrow('重复')
    expect(() => compositionPayload([{ ...a, label: '1000' }])).toThrow('字母')
    expect(() => compositionPayload([{ ...a, label: 'A/B' }])).toThrow('字母')
    expect(nextProblemLabel(['A', 'C'])).toBe('B')
    expect(
      nextProblemLabel(Array.from({ length: 26 }, (_, i) => String.fromCharCode(65 + i))),
    ).toBe('AA')
  })
  it('closes preparation controls for editors at the start but preserves owner control', () => {
    const contest = createFixtures().contests[0]
    contest.permissions.edit = true
    contest.permissions.manageAccess = false
    expect(canPrepareContest(contest, Date.parse(contest.beginAt) - 1)).toBe(true)
    expect(canPrepareContest(contest, Date.parse(contest.beginAt))).toBe(false)
    contest.permissions.manageAccess = true
    expect(canPrepareContest(contest, Date.parse(contest.endAt) + 1)).toBe(true)
    contest.permissions.edit = false
    expect(canPrepareContest(contest)).toBe(false)
  })
})
