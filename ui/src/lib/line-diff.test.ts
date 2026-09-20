import { describe, expect, it } from 'vitest'
import { lineDiff } from './line-diff'

describe('line review comparison', () => {
  it('preserves repeated lines, blank lines, and final-newline changes', () => {
    const texts = ['', 'a', 'a\n', 'a\na\nb', 'a\nb\na', '\n\n', 'a\n\nb\n']
    for (const before of texts)
      for (const after of texts) {
        const result = lineDiff(before, after)
        expect(result.truncated).toBe(false)
        expect(
          result.lines
            .filter((line) => line.kind !== 'added')
            .map((line) => line.text)
            .join('\n'),
        ).toBe(before)
        expect(
          result.lines
            .filter((line) => line.kind !== 'removed')
            .map((line) => line.text)
            .join('\n'),
        ).toBe(after)
      }
    expect(lineDiff('a\na', 'a\na\na').lines.filter((line) => line.kind === 'added')).toHaveLength(
      1,
    )
  })
  it('keeps actual line numbers around separate edits', () => {
    const result = lineDiff('one\ntwo\nthree\nfour', 'one\nnew\nthree\nfour\nfive')
    expect(result.lines.find((line) => line.text === 'two')).toMatchObject({
      kind: 'removed',
      before: 2,
    })
    expect(result.lines.find((line) => line.text === 'new')).toMatchObject({
      kind: 'added',
      after: 2,
    })
    expect(result.lines.find((line) => line.text === 'three')).toMatchObject({
      kind: 'same',
      before: 3,
      after: 3,
    })
    expect(result.lines.find((line) => line.text === 'five')).toMatchObject({
      kind: 'added',
      after: 5,
    })
  })
  it('bounds worst-case comparisons and labels incomplete previews', () => {
    const before = Array.from({ length: 1500 }, (_, i) => `a${i}`).join('\n'),
      after = Array.from({ length: 1500 }, (_, i) => `b${i}`).join('\n')
    const result = lineDiff(before, after)
    expect(result.coarse).toBe(true)
    expect(result.truncated).toBe(false)
    expect(
      result.lines
        .filter((line) => line.kind !== 'added')
        .map((line) => line.text)
        .join('\n'),
    ).toBe(before)
    expect(lineDiff('x\n'.repeat(10000), 'y\n'.repeat(10000))).toMatchObject({
      truncated: true,
      coarse: true,
    })
  })
})
