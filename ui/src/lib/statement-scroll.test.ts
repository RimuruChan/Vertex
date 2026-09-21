import { describe, expect, it } from 'vitest'
import { linkedScrollTop, previewSourceLines, scrollAnchors } from './statement-scroll'

describe('statement scroll correspondence', () => {
  it('anchors expanded samples at their source placeholder without shifting later sections', () => {
    const source = '# Sum\n\n{{remainingsamples}}\n\n## Explanation\nText'
    const rendered =
      '# Sum\n\n## Sample 1\n```\n1 2\n```\n\n## Sample 2\n```\n3 4\n```\n\n## Explanation\nText'
    const map = previewSourceLines(source, rendered)
    expect(map[0]).toBe(0)
    expect(map[2]).toBe(2)
    expect(map[7]).toBe(2)
    expect(map[12]).toBe(4)
    expect(map[13]).toBe(5)
  })
  it('keeps source lines intact without sample expansion', () => {
    expect(previewSourceLines('# Title\n\n$$x$$', '# Title\n\n$$x$$')).toEqual([0, 1, 2])
  })
  it('keeps inserted samples attached across a preserved blank line', () => {
    const source = '# Title\n\n{{remainingsamples}}\n\n## Explanation\nText'
    const rendered = '# Title\n\n\n## Sample\nInput\nOutput\n\n## Explanation\nText'
    const map = previewSourceLines(source, rendered)
    expect(map[3]).toBe(2)
    expect(map[7]).toBe(4)
  })
  it('interpolates between paragraphs in both directions rather than by total height', () => {
    const points = scrollAnchors(
      [
        { source: 100, preview: 400 },
        { source: 200, preview: 800 },
      ],
      500,
      1000,
    )
    expect(linkedScrollTop(points, 'source', 150)).toBe(600)
    expect(linkedScrollTop(points, 'preview', 600)).toBe(150)
    expect(linkedScrollTop(points, 'source', 0)).toBe(0)
    expect(linkedScrollTop(points, 'preview', 1000)).toBe(500)
  })
  it('ignores duplicate and unreachable anchors and handles panes that fit without scrolling', () => {
    const points = scrollAnchors(
      [
        { source: 100, preview: 200 },
        { source: 100, preview: 400 },
        { source: 300, preview: 1100 },
      ],
      500,
      1000,
    )
    expect(points).toEqual([
      { source: 0, preview: 0 },
      { source: 100, preview: 200 },
      { source: 500, preview: 1000 },
    ])
    expect(linkedScrollTop(scrollAnchors([], 0, 200), 'preview', 80)).toBe(0)
    expect(linkedScrollTop(points, 'source', 900)).toBe(1000)
  })
})
