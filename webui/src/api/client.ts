import {
  postApiAuthLogin,
  postApiAuthLogout,
  postApiAuthRefresh,
  postApiAuthRegister,
} from '../generated/api/vertex'
import type { DtoAuthResponse, DtoUserResponse } from '../generated/api/model'
import { onSessionRefresh, refreshSession, setAccessToken, setSessionRefresher } from './http'

type SessionListener = (user: DtoUserResponse | null) => void

let user: DtoUserResponse | null = null
let restorePromise: Promise<DtoUserResponse | null> | null = null
const sessionListeners = new Set<SessionListener>()

function updateSession(result: DtoAuthResponse | null) {
  user = result?.user ?? null
  setAccessToken(result?.accessToken ?? null)
  for (const listener of sessionListeners) listener(user)
}

onSessionRefresh(updateSession)
setSessionRefresher(() => postApiAuthRefresh({ skipAuthRefresh: true }))

export function subscribeSession(listener: SessionListener) {
  sessionListeners.add(listener)
  return () => {
    sessionListeners.delete(listener)
  }
}

export function currentUser(): DtoUserResponse | null {
  return user
}

export async function restoreSession(): Promise<DtoUserResponse | null> {
  if (!restorePromise) {
    restorePromise = refreshSession()
      .then((result) => result.user)
      .catch(() => {
        return null
      })
      .finally(() => {
        restorePromise = null
      })
  }
  return restorePromise
}

export async function register(username: string, email: string, password: string) {
  const result = await postApiAuthRegister({ username, email, password }, { skipAuthRefresh: true })
  updateSession(result)
}

export async function login(username: string, password: string) {
  const result = await postApiAuthLogin({ username, password }, { skipAuthRefresh: true })
  updateSession(result)
}

export async function logout() {
  try {
    await postApiAuthLogout()
  } finally {
    updateSession(null)
  }
}
