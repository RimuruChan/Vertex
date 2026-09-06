import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatRatio, formatRelative } from './format'

describe('formatRelative', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-04T12:00:00Z'))
  })

  afterEach(() => vi.useRealTimers())

  it('describes both past and future times without calling future events just now', () => {
    expect(formatRelative('2026-09-04T12:00:00Z')).toBe('刚刚')
    expect(formatRelative('2026-09-04T11:59:30Z')).toBe('不到 1 分钟前')
    expect(formatRelative('2026-09-04T12:00:30Z')).toBe('不到 1 分钟后')
    expect(formatRelative('2026-09-04T12:01:30Z')).toBe('1 分钟后')
    expect(formatRelative('2026-09-04T10:00:00Z')).toBe('2 小时前')
    expect(formatRelative('2026-09-07T12:00:00Z')).toBe('3 天后')
  })
})

describe('formatRatio', () => {
  it('handles empty totals and rounds ordinary ratios consistently', () => {
    expect(formatRatio(0, 0)).toBe('—')
    expect(formatRatio(1, 3)).toBe('33.3%')
  })
})
