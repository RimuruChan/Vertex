import type { AxiosRequestConfig } from 'axios'
import type { DtoAuthResponse } from '../generated/api/model'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const axiosMock = vi.hoisted(() => ({
  create: vi.fn(),
  request: vi.fn<(config: AxiosRequestConfig) => Promise<{ data: unknown }>>(),
}))

vi.mock('axios', () => ({
  default: { create: axiosMock.create },
}))

type HTTPModule = typeof import('./http')

let http: HTTPModule

beforeEach(async () => {
  vi.resetModules()
  axiosMock.create.mockReset()
  axiosMock.request.mockReset()
  axiosMock.create.mockReturnValue({ request: axiosMock.request })
  http = await import('./http')
})

describe('authenticated HTTP requests', () => {
  it('shares one refresh across concurrent 401 responses and retries both with the new token', async () => {
    const refresh = deferred<DtoAuthResponse>()
    const refreshSession = vi.fn(() => refresh.promise)
    const published = vi.fn()

    axiosMock.request.mockImplementation(async (config) => {
      const authorization = readAuthorization(config)
      if (authorization !== 'Bearer fresh-token') throw unauthorized()
      return { data: config.url }
    })
    http.setAccessToken('expired-token')
    http.setSessionRefresher(refreshSession)
    http.onSessionRefresh(published)

    const first = http.request<string>({ url: '/first' })
    const second = http.request<string>({ url: '/second' })

    await vi.waitFor(() => expect(refreshSession).toHaveBeenCalledOnce())
    refresh.resolve(authResponse('fresh-token'))

    await expect(Promise.all([first, second])).resolves.toEqual(['/first', '/second'])
    expect(refreshSession).toHaveBeenCalledOnce()
    expect(published).toHaveBeenCalledOnce()
    expect(
      axiosMock.request.mock.calls.filter(
        ([config]) => readAuthorization(config) === 'Bearer fresh-token',
      ),
    ).toHaveLength(2)
  })

  it('does not refresh a request that explicitly opts out', async () => {
    const error = unauthorized()
    const refreshSession = vi.fn(async () => authResponse('fresh-token'))
    axiosMock.request.mockRejectedValue(error)
    http.setAccessToken('expired-token')
    http.setSessionRefresher(refreshSession)

    await expect(http.request({ url: '/auth/refresh' }, { skipAuthRefresh: true })).rejects.toBe(
      error,
    )
    expect(refreshSession).not.toHaveBeenCalled()
    expect(axiosMock.request).toHaveBeenCalledOnce()
  })

  it('publishes a cleared session and does not retry when refresh fails', async () => {
    const refreshError = new Error('refresh failed')
    const refreshSession = vi.fn(async () => {
      throw refreshError
    })
    const published = vi.fn()
    axiosMock.request.mockRejectedValueOnce(unauthorized())
    http.setAccessToken('expired-token')
    http.setSessionRefresher(refreshSession)
    http.onSessionRefresh(published)

    await expect(http.request({ url: '/private' })).rejects.toBe(refreshError)
    expect(refreshSession).toHaveBeenCalledOnce()
    expect(axiosMock.request).toHaveBeenCalledOnce()
    expect(published).toHaveBeenCalledOnce()
    expect(published).toHaveBeenCalledWith(null)

    axiosMock.request.mockResolvedValueOnce({ data: 'anonymous' })
    await expect(http.request<string>({ url: '/public' }, { skipAuthRefresh: true })).resolves.toBe(
      'anonymous',
    )
    expect(readAuthorization(axiosMock.request.mock.calls[1][0])).toBeUndefined()
  })
})

function readAuthorization(config: AxiosRequestConfig): string | undefined {
  const headers = config.headers as Record<string, string> | undefined
  return headers?.Authorization
}

function unauthorized() {
  return { response: { status: 401 } }
}

function authResponse(accessToken: string): DtoAuthResponse {
  return {
    accessToken,
    expiresIn: 900,
    token: accessToken,
    user: { id: 'user-1', username: 'alice', email: 'alice@example.com', role: 'user' },
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}
