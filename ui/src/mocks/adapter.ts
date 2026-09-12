import {
  AxiosError,
  CanceledError,
  type AxiosAdapter,
  type InternalAxiosRequestConfig,
} from 'axios'
import { setRequestAdapter } from '@/api/http'
import { createMockAPI, MockError, type MockScenario } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser, mockIdentities } from './identities'
import { ensureContestExamples } from './contest-examples'

const STORAGE_KEY = 'vertex-mock:v2'
const LEGACY_STORAGE_KEY = 'vertex-mock:v1'

function load() {
  try {
    const saved = JSON.parse(
      localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_STORAGE_KEY) || 'null',
    )
    if (
      (saved?.version === 1 || saved?.version === 2) &&
      saved.state &&
      ['problems', 'submissions', 'contests', 'editorials', 'discussions', 'sets'].every((key) =>
        Array.isArray(saved.state[key]),
      ) &&
      saved.state.pending &&
      saved.state.clarifications
    ) {
      if (Array.isArray(saved.state.registrations))
        saved.state.registrations = { [demoUser.id]: saved.state.registrations }
      if (
        saved.version === 1 &&
        saved.state.user?.id === demoUser.id &&
        saved.state.user?.role === 'admin'
      )
        saved.state.user = { ...adminUser }
      return saved
    }
  } catch {
    /* Unavailable or invalid storage starts a fresh, in-memory demo. */
  }
  return null
}

const saved = load()
if (saved?.state && !saved.state.workspaces) saved.state.workspaces = {}
const initialState = saved?.state ?? createFixtures()
const addedExamples = ensureContestExamples(initialState)
export const mockAPI = createMockAPI(initialState)
if (['normal', 'slow', 'empty', 'error'].includes(saved?.scenario))
  mockAPI.scenario = saved.scenario
if (
  ['Accepted', 'Wrong Answer', 'Time Limit Exceeded', 'Compile Error'].includes(saved?.nextVerdict)
)
  mockAPI.nextVerdict = saved.nextVerdict
if (addedExamples) persistMock()

export function persistMock() {
  try {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        version: 2,
        state: mockAPI.state,
        scenario: mockAPI.scenario,
        nextVerdict: mockAPI.nextVerdict,
      }),
    )
  } catch {
    /* Storage quota/private mode does not prevent in-memory interaction. */
  }
}

export function resetMock() {
  try {
    localStorage.removeItem(STORAGE_KEY)
    localStorage.removeItem(LEGACY_STORAGE_KEY)
    for (const key of Object.keys(localStorage)) {
      if (key.startsWith('vertex-mock-draft:')) localStorage.removeItem(key)
    }
  } catch {
    /* A reload also resets an in-memory-only demo. */
  }
  window.location.reload()
}

export function changeScenario(value: MockScenario) {
  mockAPI.scenario = value
  persistMock()
  window.location.reload()
}

export function changeMockIdentity(id: string) {
  const identity = mockIdentities.find((item) => (item.user?.id ?? 'guest') === id)
  if (!identity) return
  mockAPI.state.user = identity.user ? { ...identity.user } : null
  persistMock()
  window.location.reload()
}

function delay(config: InternalAxiosRequestConfig, ms: number) {
  return new Promise<void>((resolve, reject) => {
    if (config.signal?.aborted) {
      reject(new CanceledError())
      return
    }
    const abort = () => {
      clearTimeout(timer)
      reject(new CanceledError())
    }
    const timer = setTimeout(() => {
      config.signal?.removeEventListener?.('abort', abort)
      resolve()
    }, ms)
    config.signal?.addEventListener?.('abort', abort, { once: true })
  })
}

export const mockAdapter: AxiosAdapter = async (config) => {
  await delay(config, mockAPI.scenario === 'slow' ? 1800 : 180)
  try {
    // Absolute URLs are deliberately unsupported: there is no network fallback.
    if (!config.url?.startsWith('/api/')) throw new MockError(501, 'Mock 模式仅支持本地演示接口。')
    const url = new URL(config.url, 'http://vertex.mock')
    const data = mockAPI.handle({
      method: (config.method ?? 'GET').toUpperCase(),
      path: url.pathname,
      params: { ...Object.fromEntries(url.searchParams), ...config.params },
      body:
        typeof config.data === 'string'
          ? JSON.parse(config.data)
          : config.data instanceof FormData
            ? Object.fromEntries(config.data.entries())
            : config.data,
    })
    persistMock()
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  } catch (error) {
    if (!(error instanceof MockError)) throw error
    throw new AxiosError(error.message, AxiosError.ERR_BAD_RESPONSE, config, undefined, {
      data: { error: error.message, code: 'mock.error' },
      status: error.status,
      statusText: error.message,
      headers: {},
      config,
    })
  }
}

export function installMock() {
  setRequestAdapter(mockAdapter)
}
