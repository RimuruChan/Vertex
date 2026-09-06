import axios, { AxiosError, type AxiosAdapter, type AxiosRequestConfig } from 'axios'
import type { DtoAuthResponse } from '../generated/api/model'

type RefreshListener = (response: DtoAuthResponse | null) => void
export type RequestOptions = AxiosRequestConfig & { skipAuthRefresh?: boolean }

const transport = axios.create({ withCredentials: true })

export function setRequestAdapter(adapter: AxiosAdapter) {
  transport.defaults.adapter = adapter
}

let accessToken: string | null = null
let refreshPromise: Promise<DtoAuthResponse> | null = null
let sessionRefresher: (() => Promise<DtoAuthResponse>) | null = null
const refreshListeners = new Set<RefreshListener>()

export function setAccessToken(token: string | null) {
  accessToken = token
}

export function onSessionRefresh(listener: RefreshListener) {
  refreshListeners.add(listener)
  return () => {
    refreshListeners.delete(listener)
  }
}

export function setSessionRefresher(refresher: () => Promise<DtoAuthResponse>) {
  sessionRefresher = refresher
}

function publishRefresh(response: DtoAuthResponse | null) {
  for (const listener of refreshListeners) listener(response)
}

export async function refreshSession(): Promise<DtoAuthResponse> {
  if (!refreshPromise) {
    if (!sessionRefresher) throw new Error('session refresher is not configured')
    refreshPromise = sessionRefresher()
      .then((result) => {
        setAccessToken(result.accessToken)
        publishRefresh(result)
        return result
      })
      .catch((error) => {
        setAccessToken(null)
        publishRefresh(null)
        throw error
      })
      .finally(() => {
        refreshPromise = null
      })
  }
  return refreshPromise
}

export async function request<T>(config: AxiosRequestConfig, options?: RequestOptions): Promise<T> {
  const merged: RequestOptions = {
    ...config,
    ...options,
    headers: { ...config.headers, ...options?.headers },
    withCredentials: true,
  }
  if (accessToken) {
    merged.headers = { ...merged.headers, Authorization: `Bearer ${accessToken}` }
  }
  const { skipAuthRefresh, ...requestConfig } = merged

  try {
    const response = await transport.request<T>(requestConfig)
    return response.data
  } catch (error) {
    const axiosError = error as AxiosError
    if (axiosError.response?.status !== 401 || skipAuthRefresh) throw error

    const refreshed = await refreshSession()
    const response = await transport.request<T>({
      ...requestConfig,
      headers: { ...requestConfig.headers, Authorization: `Bearer ${refreshed.accessToken}` },
    })
    return response.data
  }
}
