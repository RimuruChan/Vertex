import { describe, expect, it } from 'vitest'
import { applyContestPreset } from './contest-presets'
import { contestPayload, newContestDraft } from './contest-form'
import { contestFormatOptions } from '@/lib/contest-formats'

describe('contest presets', () => {
  it.each(contestFormatOptions)(
    'applies %s without changing identity, schedule or admission',
    (format) => {
      const original = {
        ...newContestDraft(),
        title: 'Preserve',
        description: 'Draft',
        allowSelfRegistration: false,
      }
      const draft = applyContestPreset(original, format)
      expect(draft.rule).toBe(format)
      expect(draft.medals).toEqual(
        format === 'icpc'
          ? { mode: 'percentage', gold: 10, silver: 20, bronze: 30 }
          : { mode: 'none', gold: 0, silver: 0, bronze: 0 },
      )
      expect(draft.feedback).toBe(
        format === 'oi'
          ? 'none'
          : format === 'icpc'
            ? 'summary'
            : format === 'cf'
              ? 'first_error'
              : 'full',
      )
      expect([
        draft.title,
        draft.description,
        draft.beginAt,
        draft.endAt,
        draft.freezeAt,
        draft.visibility,
        draft.allowSelfRegistration,
      ]).toEqual([
        original.title,
        original.description,
        original.beginAt,
        original.endAt,
        original.freezeAt,
        original.visibility,
        original.allowSelfRegistration,
      ])
      expect(original.rule).toBe('icpc')
    },
  )
  it('switching away from OI restores feedback and clears ICPC-only penalty settings', () => {
    const oi = applyContestPreset(newContestDraft(), 'oi')
    const cf = applyContestPreset(oi, 'cf')
    expect(cf).toMatchObject({
      feedback: 'first_error',
      penaltyMinutes: 0,
      penalizeCompileError: false,
    })
  })
  it('cannot submit an OI configuration with live feedback', () => {
    expect(
      contestPayload({ ...newContestDraft(), title: 'OI', rule: 'oi', feedback: 'full' }).feedback,
    ).toBe('none')
  })
})
