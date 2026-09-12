import { describe, expect, it } from 'vitest'
import { calendarDays, localDateText, parseLocalDateTime, shiftLocalMinutes } from './date-time'

describe('local date/time picker', () => {
  it('builds exclusive minute bounds across day boundaries', () => {
    expect(shiftLocalMinutes('2026-09-09T23:59', 1)).toBe('2026-09-10T00:00')
    expect(shiftLocalMinutes('2026-09-09T00:00', -1)).toBe('2026-09-08T23:59')
    expect(shiftLocalMinutes('', 1)).toBe('')
  })
  it('preserves local wall-clock time without UTC conversion', () => {
    const date = parseLocalDateTime('2026-09-09T17:35')!
    expect(localDateText(date)).toBe('2026-09-09')
    expect(date.getHours()).toBe(17)
    expect(date.getMinutes()).toBe(35)
  })
  it('rejects date/time rollover instead of silently changing the selection', () => {
    for (const value of [
      '',
      '2026-02-29T12:00',
      '2026-04-31T12:00',
      '2026-13-01T12:00',
      '2026-09-09T24:00',
      '2026-09-09T12:60',
    ]) {
      expect(parseLocalDateTime(value)).toBeNull()
    }
    expect(parseLocalDateTime('2028-02-29T00:00')).not.toBeNull()
  })
  it('builds six complete Monday-first weeks across month boundaries', () => {
    const days = calendarDays(new Date(2026, 8, 9))
    expect(days).toHaveLength(42)
    expect(days[0].getDay()).toBe(1)
    expect(localDateText(days[0])).toBe('2026-08-31')
    expect(localDateText(days[41])).toBe('2026-10-11')
    expect(new Set(days.map(localDateText)).size).toBe(42)
  })
})
