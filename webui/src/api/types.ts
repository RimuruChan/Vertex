// 与后端 API 对齐的类型定义。

export interface User {
  id: string
  username: string
  email: string
  role: 'user' | 'admin'
}

export interface AuthResponse {
  token: string
  user: User
}

export interface Problem {
  id: string
  title: string
  statementMd: string
  difficulty: number
  source: string
  timeLimitMs: number
  memoryLimitKb: number
  visibility: 'draft' | 'private' | 'public'
  authorId?: string
  submissionCount: number
  acceptedCount: number
  solvedUserCount: number
  judgeType: 'normal' | 'interactive'
  tags: string[]
  createdAt: string
  updatedAt: string
}

export interface CaseResult {
  caseIndex: number
  verdict: string
  timeMs: number
  memoryKb: number
  exitStatus?: string
  checkerOutput?: string
}

export interface Submission {
  id: string
  userId: string
  username?: string
  problemId: string
  problemTitle?: string
  language: string
  sourceCode?: string
  status: string
  score: number
  totalTimeMs: number
  peakMemoryKb: number
  compileResult?: string
  caseResults?: CaseResult[]
  contestId?: string
  submittedAt: string
  judgedAt?: string
}

export interface ListResponse<T> {
  items: T[]
  total: number
}

export interface Contest {
  id: string
  title: string
  description: string
  rule: 'acm' | 'ioi'
  beginAt: string
  endAt: string
  freezeAt?: string
  visibility: 'public' | 'private' | 'password'
  rankboardVisible: boolean
  createdBy?: string
  createdAt: string
}

export interface ContestProblem {
  contestId: string
  problemId: string
  sortOrder: number
}

export interface Editorial {
  id: string
  problemId: string
  authorId?: string
  authorName?: string
  title: string
  contentMd: string
  visibility: string
  status: string
  createdAt: string
  updatedAt: string
}

export interface DiscussionPost {
  id: number
  problemId?: string
  editorialId?: string
  contestId?: string
  authorId?: string
  authorName?: string
  contentMd: string
  parentId?: number
  createdAt: string
}

// ACM 榜单类型
export interface ACMCell {
  attempts: number
  penaltySec: number
  solvedAt?: string
  pendingCount: number
}

export interface RankRow {
  rank: number
  username: string
  userId: string
  solved: number
  penalty: number
  cells: ACMCell[]
  hasFreezeHit: boolean
}

export interface Rankboard {
  problemCount: number
  problemIds: string[]
  rows: RankRow[]
  frozen: boolean
  frozenAt?: string
}
