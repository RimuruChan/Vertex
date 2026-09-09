import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, contestantUser, juryUser, observerUser } from './identities'

describe('contest wire visibility', () => {
  it('omits metadata in list and detail for every contest role, restoring it only when allowed', () => {
    const now = Date.parse('2030-01-01T12:00:00Z')
    const state = createFixtures(now)
    const event = state.contests[0]
    event.beginAt = new Date(now - 3600000).toISOString()
    event.endAt = new Date(now + 3600000).toISOString()
    event.showProblemMetadata = false
    const api = createMockAPI(state, () => now)
    for (const user of [adminUser, juryUser, observerUser, contestantUser]) {
      state.user = { ...user }
      const details = api.handle({ method: 'GET', path: `/api/contests/${event.publicId}` }) as {
        problems: { label: string }[]
      }
      expect(details.problems.length).toBeGreaterThan(0)
      expect(details.problems[0]).not.toHaveProperty('difficulty')
      expect(details.problems[0]).not.toHaveProperty('tags')
      const problem = api.handle({
        method: 'GET',
        path: `/api/contests/${event.publicId}/problems/${details.problems[0].label}`,
      })
      expect(problem).not.toHaveProperty('difficulty')
      expect(problem).not.toHaveProperty('tags')
    }
    event.showProblemMetadata = true
    const visible = api.handle({ method: 'GET', path: `/api/contests/${event.publicId}` }) as {
      problems: unknown[]
    }
    expect(visible.problems[0]).toHaveProperty('difficulty')
    event.showProblemMetadata = false
    event.endAt = new Date(now - 1).toISOString()
    const ended = api.handle({ method: 'GET', path: `/api/contests/${event.publicId}` }) as {
      problems: unknown[]
    }
    expect(ended.problems[0]).toHaveProperty('tags')
  })

  it('does not disclose no-feedback scores through the public scoreboard', () => {
    const now = Date.parse('2030-01-01T12:00:00Z')
    const state = createFixtures(now)
    const event = state.contests[0]
    event.beginAt = new Date(now - 3600000).toISOString()
    event.endAt = new Date(now + 3600000).toISOString()
    event.feedback = 'none'
    const api = createMockAPI(state, () => now)
    state.user = { ...contestantUser }
    expect(() =>
      api.handle({
        method: 'GET',
        path: `/api/contests/${event.publicId}/rankboard`,
        params: { view: 'jury' },
      }),
    ).toThrow()
    state.user = { ...observerUser }
    expect(() =>
      api.handle({ method: 'GET', path: `/api/contests/${event.publicId}/rankboard` }),
    ).toThrow()
    expect(
      api.handle({
        method: 'GET',
        path: `/api/contests/${event.publicId}/rankboard`,
        params: { view: 'jury' },
      }),
    ).toHaveProperty('juryView', true)
  })
})
