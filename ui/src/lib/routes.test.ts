import { describe, expect, it } from 'vitest'
import { matchesReference, problemHref, submissionHref } from './routes'

describe('public routes', () => {
  it('keeps contest submissions inside the contest while preserving practice routes', () => {
    expect(submissionHref({ id: 's', publicId: '25' })).toBe('/submissions/25')
    expect(submissionHref({ id: 's', publicId: '25', contestId: 'c', contestPublicId: '3' })).toBe(
      '/contests/3/submissions/25',
    )
    expect(submissionHref({ id: 's', contestId: 'c' })).toBe('/contests/c/submissions/s')
  })
  it('uses public numbers for practice and preserves the contest scope', () => {
    expect(problemHref({ problemId: 'problem-uuid', problemPublicId: '1000' })).toBe(
      '/problems/1000',
    )
    expect(
      problemHref({
        problemId: 'problem-uuid',
        problemPublicId: '1000',
        contestId: 'contest-uuid',
        contestPublicId: '42',
        label: 'A',
      }),
    ).toBe('/contests/42/problems/A')
    expect(
      problemHref({
        problemId: 'problem-uuid',
        problemPublicId: '1000',
        contestId: 'contest-uuid',
        contestPublicId: '42',
      }),
    ).toBe('/contests/42/problems/1000')
  })
  it('does not canonicalize a stale response from a previously opened resource', () => {
    const current = { id: 'old-uuid', publicId: '1000' }
    expect(matchesReference('old-uuid', current)).toBe(true)
    expect(matchesReference('1000', current)).toBe(true)
    expect(matchesReference('1001', current)).toBe(false)
    expect(matchesReference(undefined, current)).toBe(false)
  })
})
