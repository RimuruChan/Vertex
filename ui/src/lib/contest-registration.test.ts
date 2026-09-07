import { describe, expect, it } from 'vitest'
import { registrationWindow } from './contest-registration'

const contest = {
  beginAt: '2030-01-01T10:00:00Z',
  endAt: '2030-01-01T12:00:00Z',
  allowSelfRegistration: true,
  allowLateRegistration: false,
}
const begin = Date.parse(contest.beginAt),
  end = Date.parse(contest.endAt)

describe('self-service registration window', () => {
  it.each([
    [true, false, begin - 1, 'open'],
    [true, false, begin, 'closed'],
    [true, true, begin, 'open'],
    [true, true, end - 1, 'open'],
    [true, true, end, 'ended'],
    [true, true, end + 1, 'ended'],
    [false, false, begin - 1, 'disabled'],
    [false, true, begin, 'disabled'],
  ] as const)(
    'self=%s late=%s at %s is %s',
    (allowSelfRegistration, allowLateRegistration, now, expected) => {
      expect(
        registrationWindow({ ...contest, allowSelfRegistration, allowLateRegistration }, now),
      ).toBe(expected)
    },
  )
})
