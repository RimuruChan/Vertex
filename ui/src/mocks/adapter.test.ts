import axios from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockAdapter, mockAPI } from './adapter'

afterEach(() => {
  vi.useRealTimers()
  mockAPI.scenario = 'normal'
})

describe('mock transport', () => {
  it('cancels an in-flight write before mutating data', async () => {
    vi.useFakeTimers()
    const client = axios.create({ adapter: mockAdapter })
    const controller = new AbortController()
    const before = mockAPI.state.submissions.length
    const result = client.post(
      '/api/submissions',
      { problemId: mockAPI.state.problems[0].id, language: 'cpp', sourceCode: 'demo' },
      { signal: controller.signal },
    )
    const assertion = expect(result).rejects.toMatchObject({ code: 'ERR_CANCELED' })
    controller.abort()
    await assertion
    await vi.runAllTimersAsync()
    expect(mockAPI.state.submissions).toHaveLength(before)
  })

  it('returns Axios-shaped failures and never falls through to an absolute URL', async () => {
    vi.useFakeTimers()
    const client = axios.create({ adapter: mockAdapter })
    const result = client.get('https://example.test/api/problems')
    const assertion = expect(result).rejects.toMatchObject({
      response: { status: 501, data: { error: expect.any(String) } },
    })
    await vi.runAllTimersAsync()
    await assertion
  })
})
