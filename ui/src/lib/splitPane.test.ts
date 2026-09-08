import { describe, expect, it } from 'vitest'
import { DIVIDER_WIDTH, MIN_PANE_WIDTH, pointerSplit, splitLimits } from './splitPane'

describe('split pane sizing', () => {
  it('snaps within twenty-four pixels of the visual center and releases outside it', () => {
    expect(pointerSplit(576, 1200)).toBe(50)
    expect(pointerSplit(624, 1200)).toBe(50)
    expect(pointerSplit(575, 1200)).toBeLessThan(50)
    expect(pointerSplit(625, 1200)).toBeGreaterThan(50)
  })

  it('keeps both panels usable at the narrow desktop breakpoint', () => {
    const width = 1000
    const available = width - DIVIDER_WIDTH
    expect((pointerSplit(-100, width) / 100) * available).toBeCloseTo(MIN_PANE_WIDTH)
    expect(((100 - pointerSplit(2000, width)) / 100) * available).toBeCloseTo(MIN_PANE_WIDTH)
  })

  it('retains the 25–75 percent range on wide screens', () => {
    expect(splitLimits(1600)).toEqual({ min: 25, max: 75 })
  })

  it('handles containers too small for two minimum-width panels', () => {
    expect(splitLimits(500)).toEqual({ min: 50, max: 50 })
    expect(pointerSplit(0, 0)).toBe(50)
  })
})
