import type {
  DtoContestResponse,
  DtoContestProblemResponse,
  DtoRankboardResponse,
  DtoRankboardCellResponse,
  DtoSubmissionResponse,
} from '@/generated/api/model'
import type { MockState } from './fixtures'
import { mockUsers } from './identities'
import { MockError } from './errors'

// Derive standings from the same simulated registrations and submissions that
// other pages display. No invented entrants or fixed score rows.
export function rankboard(
  state: MockState,
  contest: DtoContestResponse,
  problems: DtoContestProblemResponse[],
  staff: boolean,
  juryRequested: boolean,
  now: number,
): DtoRankboardResponse {
  const begin = Date.parse(contest.beginAt)
  if (contest.feedback === 'none' && now <= Date.parse(contest.endAt) && !(staff && juryRequested))
    throw new MockError(403, '比赛不反馈判定，公开榜单将在赛后开放')
  const enrolled = (state.registrations[state.user?.id ?? ''] ?? []).includes(contest.id)
  if (
    !staff &&
    (now < begin || !contest.rankboardVisible || (contest.visibility === 'password' && !enrolled))
  )
    throw new MockError(403, '榜单尚未开放或需要报名')
  const scheduledFreeze =
    !!contest.freezeAt &&
    now > Date.parse(contest.freezeAt) &&
    (!contest.unfreezeAt || now < Date.parse(contest.unfreezeAt))
  const frozen = scheduledFreeze && !(staff && juryRequested)
  const rows = Object.entries(state.registrations)
    .filter(([, events]) => events.includes(contest.id))
    .flatMap(([userId]) => {
      const user =
        mockUsers.find((u) => u.id === userId) ??
        (state.user?.id === userId ? state.user : undefined)
      if (!user) return []
      const cells = problems.map((problem) =>
        scoreCell(
          contest,
          problem,
          state.submissions.filter(
            (s) =>
              s.contestId === contest.id &&
              s.userId === userId &&
              s.problemId === problem.problemId,
          ),
          frozen,
        ),
      )
      const times = cells.flatMap((cell) => (cell.solvedAt ? [cell.solvedAt] : [])).sort()
      return [
        {
          rank: 0,
          userId,
          username: user.username,
          cells,
          solved: cells.filter((cell) => cell.solvedAt).length,
          score: cells.reduce((total, cell) => total + cell.score, 0),
          penalty: cells.reduce((total, cell) => total + (cell.solvedAt ? cell.penaltySec : 0), 0),
          hasPending: cells.some((cell) => cell.pendingCount > 0),
          lastAcceptedAt: times.at(-1),
        },
      ]
    })
  const format = contest.format
  const compare = (a: (typeof rows)[number], b: (typeof rows)[number]) =>
    format === 'icpc'
      ? b.solved - a.solved || a.penalty - b.penalty
      : b.score - a.score || (a.lastAcceptedAt ?? '9999').localeCompare(b.lastAcceptedAt ?? '9999')
  rows.sort(
    (a, b) => compare(a, b) || (a.username < b.username ? -1 : a.username > b.username ? 1 : 0),
  )
  rows.forEach((row, i) => {
    const previous = rows[i - 1]
    row.rank =
      previous &&
      (format === 'icpc'
        ? previous.solved === row.solved && previous.penalty === row.penalty
        : previous.score === row.score)
        ? previous.rank
        : i + 1
  })
  for (let column = 0; column < problems.length; column++) {
    const first = rows
      .filter((row) => row.cells[column].solvedAt)
      .sort((a, b) => a.cells[column].solvedAt!.localeCompare(b.cells[column].solvedAt!))[0]
    if (first) first.cells[column].firstSolver = true
  }
  return {
    format,
    frozen,
    juryView: staff && !frozen,
    frozenAt: contest.freezeAt,
    unfreezeAt: contest.unfreezeAt,
    problemCount: problems.length,
    problemIds: problems.map((p) => p.problemId),
    problems,
    rows,
  }
}

function scoreCell(
  contest: DtoContestResponse,
  problem: DtoContestProblemResponse,
  submissions: DtoSubmissionResponse[],
  frozen: boolean,
): DtoRankboardCellResponse {
  const begin = Date.parse(contest.beginAt),
    end = Date.parse(contest.endAt),
    freeze = Date.parse(contest.freezeAt ?? '')
  const eligible = submissions
    .filter((s) => {
      const time = Date.parse(s.submittedAt)
      return (
        time >= begin &&
        time <= end &&
        !['Pending', 'Judging', 'System Error', ''].includes(s.status) &&
        (s.status !== 'Compile Error' || contest.penalizeCompileError)
      )
    })
    .sort((a, b) => a.submittedAt.localeCompare(b.submittedAt))
  const chosen = frozen ? eligible.filter((s) => Date.parse(s.submittedAt) < freeze) : eligible
  const result: DtoRankboardCellResponse = {
    attempts: 0,
    score: 0,
    penaltySec: 0,
    firstSolver: false,
    pendingCount: frozen ? eligible.length - chosen.length : 0,
  }
  const points = (s: DtoSubmissionResponse) =>
    Math.floor((Math.max(0, Math.min(100, s.score)) * problem.points) / 100)
  if (contest.format === 'icpc') {
    for (const s of chosen) {
      result.attempts++
      if (s.status === 'Accepted') {
        result.score = problem.points
        result.solvedAt = s.submittedAt
        result.penaltySec =
          Math.max(0, Math.floor((Date.parse(s.submittedAt) - begin) / 1000)) +
          (result.attempts - 1) * contest.penaltyMinutes * 60
        break
      }
    }
  } else {
    result.attempts = chosen.length
    const selected =
      contest.format === 'oi'
        ? chosen.at(-1)
        : chosen.reduce<DtoSubmissionResponse | undefined>(
            (best, s) => (!best || points(s) > points(best) ? s : best),
            undefined,
          )
    if (selected) {
      result.score = points(selected)
      if (result.score > 0 && result.score >= problem.points) result.solvedAt = selected.submittedAt
    }
  }
  return result
}
