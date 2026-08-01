import axios from 'axios'
import type { AuthResponse, Contest, DiscussionPost, Editorial, ListResponse, Problem, Rankboard, Submission, User } from './types'

// API 客户端:从 localStorage 读取 token,401 时清除。
const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('vertex_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('vertex_token')
      localStorage.removeItem('vertex_user')
    }
    return Promise.reject(err)
  },
)

// ---------- Auth ----------
export async function register(username: string, email: string, password: string): Promise<AuthResponse> {
  const { data } = await api.post('/auth/register', { username, email, password })
  localStorage.setItem('vertex_token', data.token)
  localStorage.setItem('vertex_user', JSON.stringify(data.user))
  return data
}

export async function login(username: string, password: string): Promise<AuthResponse> {
  const { data } = await api.post('/auth/login', { username, password })
  localStorage.setItem('vertex_token', data.token)
  localStorage.setItem('vertex_user', JSON.stringify(data.user))
  return data
}

export function logout() {
  localStorage.removeItem('vertex_token')
  localStorage.removeItem('vertex_user')
}

export function currentUser(): User | null {
  const raw = localStorage.getItem('vertex_user')
  if (!raw) return null
  try {
    return JSON.parse(raw) as User
  } catch {
    return null
  }
}

export async function me(): Promise<User> {
  const { data } = await api.get('/auth/me')
  return data
}

// ---------- Problems ----------
export async function listProblems(params: {
  page?: number
  size?: number
  difficulty?: number
  tag?: string
  keyword?: string
}): Promise<ListResponse<Problem>> {
  const { data } = await api.get('/problems', { params })
  return data
}

export async function getProblem(id: string): Promise<Problem> {
  const { data } = await api.get(`/problems/${id}`)
  return data
}

// ---------- Submissions ----------
export async function submit(params: {
  problemId: string
  language: string
  sourceCode: string
}): Promise<Submission> {
  const { data } = await api.post('/submissions', params)
  return data
}

export async function listSubmissions(params: {
  page?: number
  size?: number
  user?: string
  problem?: string
  contest?: string
  language?: string
  status?: string
}): Promise<ListResponse<Submission>> {
  const { data } = await api.get('/submissions', { params })
  return data
}

export async function getSubmission(id: string): Promise<Submission> {
  const { data } = await api.get(`/submissions/${id}`)
  return data
}

// ---------- Admin: problems ----------
export async function adminListProblems(params: {
  page?: number
  size?: number
  visibility?: string
  keyword?: string
}): Promise<ListResponse<Problem>> {
  const { data } = await api.get('/admin/problems', { params })
  return data
}

export async function adminCreateProblem(input: Partial<Problem>): Promise<Problem> {
  const { data } = await api.post('/admin/problems', input)
  return data
}

export async function adminUpdateProblem(id: string, input: Partial<Problem>): Promise<Problem> {
  const { data } = await api.put(`/admin/problems/${id}`, input)
  return data
}

export async function adminDeleteProblem(id: string): Promise<void> {
  await api.delete(`/admin/problems/${id}`)
}

export async function adminUploadTestdata(id: string, file: File, checker: string): Promise<{ caseCount: number; sha256: string }> {
  const form = new FormData()
  form.append('file', file)
  form.append('checker', checker)
  const { data } = await api.post(`/admin/problems/${id}/testdata`, form)
  return data
}

export async function adminRejudge(id: string): Promise<void> {
  await api.post(`/admin/submissions/${id}/rejudge`)
}

// ---------- Contests (M4) ----------
export async function listContests(): Promise<ListResponse<Contest>> {
  const { data } = await api.get('/contests')
  return data
}

export async function getContest(id: string): Promise<{ contest: Contest; problems: { problemId: string; sortOrder: number }[] }> {
  const { data } = await api.get(`/contests/${id}`)
  return data
}

export async function registerContest(id: string): Promise<void> {
  await api.post(`/contests/${id}/register`)
}

export async function getContestRankboard(id: string, frozen?: boolean): Promise<Rankboard> {
  const { data } = await api.get(`/contests/${id}/rankboard`, { params: frozen === undefined ? {} : { frozen } })
  return data
}

export async function adminCreateContest(input: Record<string, unknown>): Promise<Contest> {
  const { data } = await api.post('/admin/contests', input)
  return data
}

export async function adminSetContestProblems(id: string, problemIds: string[]): Promise<void> {
  await api.put(`/admin/contests/${id}/problems`, { problemIds })
}

// ---------- Editorials / Discussions (M5) ----------
export async function listEditorials(problemId: string): Promise<ListResponse<Editorial>> {
  const { data } = await api.get('/editorials', { params: { problem: problemId } })
  return data
}

export async function createEditorial(input: { problemId: string; title: string; contentMd: string }): Promise<Editorial> {
  const { data } = await api.post('/editorials', input)
  return data
}

export async function listProblemDiscussions(problemId: string): Promise<ListResponse<DiscussionPost>> {
  const { data } = await api.get(`/problems/${problemId}/discussions`)
  return data
}

export async function createProblemDiscussion(problemId: string, contentMd: string, parentId?: number): Promise<DiscussionPost> {
  const { data } = await api.post(`/problems/${problemId}/discussions`, { contentMd, parentId })
  return data
}

export async function deleteDiscussion(id: number): Promise<void> {
  await api.delete(`/discussions/${id}`)
}

export default api
