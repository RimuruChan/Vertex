import { contestFormatDescription, contestFormatName } from '@/lib/contest-formats'
import type { DtoContestResponse, DtoSubmissionResponse } from '@/generated/api/model'
import { mockID, type MockState } from './fixtures'
import {
  adminUser,
  contestantUser,
  demoUser,
  juryUser,
  observerUser,
  scoreboardUsers,
} from './identities'
import { officialDomainID } from './problem-permissions'
import { contestPermissions } from './contest-permissions'

/** Add once to both fresh and persisted browser demos, without replacing user edits. */
export function ensureContestExamples(state: MockState, now = Date.now()) {
  if (state.scoreboardExamplesVersion === 2) return false
  const problems = state.problems.filter((p) => p.publishedVersion > 0).slice(0, 8)
  if (problems.length < 3) return false
  const minute = 60000
  const begin = now - 90 * minute
  const entrants = [contestantUser, demoUser, ...scoreboardUsers]
  const colors = [
    '#ef4444',
    '#f59e0b',
    '#22c55e',
    '#3b82f6',
    '#a855f7',
    '#ec4899',
    '#14b8a6',
    '#64748b',
  ]
  let nextContest = Math.max(0, ...state.contests.map((c) => Number(c.publicId) || 0)) + 1
  let nextSubmission = Math.max(0, ...state.submissions.map((s) => Number(s.publicId) || 0)) + 1
  let serial =
    Math.max(
      0,
      ...state.submissions
        .filter((s) => s.id.startsWith('9400'))
        .map((s) => Number(s.id.slice(4, 8))),
    ) + 1
  state.contestEntries ??= {}
  for (const [formatIndex, format] of (['icpc', 'oi', 'ioi', 'leduo', 'cf'] as const).entries()) {
    for (const frozen of [false, true]) {
      const index = formatIndex * 2 + Number(frozen) + 1
      const id = mockID(9300, index)
      const offset = format === 'oi' && frozen ? -240 * minute : 0
      const at = (minutes: number) => new Date(begin + offset + minutes * minute).toISOString()
      const title = `${contestFormatName(format)} 榜单演示 · ${format === 'oi' ? (frozen ? '赛后公布' : '赛中不反馈') : frozen ? '封榜' : '未封榜'}`
      const description = `${entrants.length} 位选手 · ${problems.length} 道题。${contestFormatDescription[format]}${format === 'oi' ? (frozen ? '本场已结束，公开榜单展示最终成绩。' : '可用裁判内部视图查看数据，选手看不到正式结果。') : frozen ? '切换公开视图查看封榜效果。' : '实时公开榜单。'}`
      const existing = state.contests.find((c) => c.id === id)
      if (existing) {
        if (format === 'oi') {
          const shift = Date.parse(at(0)) - Date.parse(existing.beginAt)
          Object.assign(existing, {
            title,
            description,
            feedback: 'none',
            beginAt: at(0),
            endAt: at(180),
            freezeAt: undefined,
            unfreezeAt: undefined,
            submissionVisibility: 'own',
          })
          for (const submission of state.submissions.filter(
            (s) => s.contestId === id && s.id.startsWith('9400'),
          )) {
            submission.submittedAt = new Date(
              Date.parse(submission.submittedAt) + shift,
            ).toISOString()
            if (submission.judgedAt)
              submission.judgedAt = new Date(Date.parse(submission.judgedAt) + shift).toISOString()
          }
        }
        continue
      }
      const details = {
        ownerId: adminUser.id,
        visibility: 'public',
        admission: 'members',
        allowSelfRegistration: true,
        allowLateRegistration: true,
        beginAt: at(0),
        endAt: at(180),
      } as const
      const contest: DtoContestResponse = {
        ...details,
        id,
        publicId: String(nextContest++),
        domainId: officialDomainID,
        title,
        description,
        ownerName: adminUser.username,
        createdAt: at(-1440),
        rule: format,
        format,
        feedback: format === 'oi' ? 'none' : 'full',
        penaltyMinutes: 20,
        penalizeCompileError: true,
        rankboardVisible: true,
        showProblemMetadata: false,
        submissionVisibility: format === 'oi' ? 'own' : 'during',
        sourceCodeVisibility: 'after_end',
        frozenSubmissionVisibility: 'pending',
        freezeAt: frozen && format !== 'oi' ? at(60) : undefined,
        permissions: contestPermissions(details, state.user, '', false, [], undefined, now),
      }
      state.contests.push(contest)
      state.contestProblemIds[id] = problems.map((p) => p.id)
      state.contestProblemVersions[id] = Object.fromEntries(
        problems.map((p) => [p.id, p.publishedVersion]),
      )
      state.contestEntries[id] = problems.map((p, i) => ({
        problemId: p.id,
        label: String.fromCharCode(65 + i),
        points:
          format === 'icpc'
            ? 100
            : format === 'cf'
              ? 500 * (i + 1)
              : [100, 100, 150, 150, 200, 200, 250, 300][i],
        color: colors[i],
      }))
      state.staff[id] = [juryUser, observerUser].map((u, i) => ({
        userId: u.id,
        username: u.username,
        role: i ? 'observer' : 'jury',
        createdAt: at(-30),
      }))
      entrants.forEach((user, row) => {
        const registrations = (state.registrations[user.id] ??= [])
        if (!registrations.includes(id)) registrations.push(id)
        // Keep the last two participants unattempted to exercise zero-score ties.
        if (row >= entrants.length - 2) return
        problems.forEach((problem, column) => {
          const pattern = row === 0 ? 3 : (row * 3 + column * 5) % 8
          const early = 8 + column * 4 + (row % 11)
          const late = 65 + column * 2 + (row % 7)
          const submit = (time: number, score: number, failure = 'Wrong Answer') => {
            const status = score === 100 ? 'Accepted' : failure
            const submission: DtoSubmissionResponse = {
              id: mockID(9400, serial++),
              publicId: String(nextSubmission++),
              contestId: id,
              contestPublicId: contest.publicId,
              problemId: problem.id,
              problemPublicId: problem.publicId!,
              problemTitle: problem.title,
              problemVersion: problem.publishedVersion,
              userId: user.id,
              username: user.username,
              language: row % 5 === 0 ? 'python' : 'cpp',
              status,
              score: format === 'icpc' && score !== 100 ? 0 : score,
              submittedAt: at(time),
              judgedAt: at(time + 0.02),
              totalTimeMs: 12 + row * 7 + column * 11,
              peakMemoryKb: 4096 + row * 256,
              judgedCases: 10,
              totalCases: 10,
              sourceCode: '// 榜单示例提交，仅用于界面演示。\n',
            }
            state.submissions.push(submission)
          }
          if (pattern === 1) submit(early, 0)
          if (pattern === 2) submit(early, 0, 'Time Limit Exceeded')
          if (pattern === 3) submit(early, 100)
          if (pattern === 4) {
            submit(early, 0)
            submit(late, 100)
          }
          if (pattern === 5) {
            submit(early, format === 'icpc' ? 0 : 30)
            submit(late, format === 'icpc' ? 100 : 75)
          }
          if (pattern === 6) {
            submit(early, 100)
            submit(late, 0)
          }
          if (pattern === 7) {
            submit(early, 0, 'Compile Error')
            submit(late, format === 'icpc' ? 0 : 50)
          }
        })
      })
    }
  }
  state.scoreboardExamplesVersion = 2
  return true
}
