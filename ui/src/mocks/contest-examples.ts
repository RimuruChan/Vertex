import { contestFormatDescription, contestFormatName } from '@/lib/contest-formats'
import { defaultMedals } from '@/lib/contest-medals'
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

const examples = [
  { index: 2, format: 'icpc', frozen: true, ended: false },
  { index: 4, format: 'oi', frozen: false, ended: true },
  { index: 5, format: 'ioi', frozen: false, ended: false },
  { index: 7, format: 'leduo', frozen: false, ended: false },
  { index: 9, format: 'cf', frozen: false, ended: false },
] as const

// Remove only redundant generated examples. Keep customized contests and any
// example containing user-created submissions, questions or rejudging work.
function removeRedundantExamples(state: MockState) {
  const redundant = [
    [1, 'icpc', 'ICPC 榜单演示 · 未封榜'],
    [3, 'oi', 'OI 榜单演示 · 赛中不反馈'],
    [6, 'ioi', 'IOI 榜单演示 · 封榜'],
    [8, 'leduo', '乐多 榜单演示 · 封榜'],
    [10, 'cf', 'CF 积分 榜单演示 · 封榜'],
  ] as const
  const remove = new Set<string>()
  for (const [index, format, title] of redundant) {
    const id = mockID(9300, index)
    const contest = state.contests.find((c) => c.id === id)
    if (
      !contest ||
      contest.title !== title ||
      contest.format !== format ||
      contest.medals ||
      contest.visibility !== 'public'
    )
      continue
    if (
      state.submissions.some((s) => s.contestId === id && !s.id.startsWith('9400')) ||
      state.clarifications[id]?.length ||
      state.rejudgeBatches.some((batch) => batch.record.contestId === id)
    )
      continue
    if (
      state.contestGrants?.[id]?.some(
        (grant) => grant.groupId || ![juryUser.id, observerUser.id].includes(grant.userId ?? ''),
      )
    )
      continue
    remove.add(id)
  }
  for (const submission of state.submissions) {
    if (!submission.contestId || !remove.has(submission.contestId)) continue
    delete state.pending[submission.id]
    delete state.submissionGenerations[submission.id]
  }
  state.submissions = state.submissions.filter((s) => !s.contestId || !remove.has(s.contestId))
  state.contests = state.contests.filter((c) => !remove.has(c.id))
  for (const user of Object.keys(state.registrations))
    state.registrations[user] = state.registrations[user].filter((id) => !remove.has(id))
  for (const id of remove) {
    delete state.contestProblemIds[id]
    delete state.contestProblemVersions[id]
    delete state.staff[id]
    delete state.clarifications[id]
    if (state.contestEntries) delete state.contestEntries[id]
    if (state.contestGrants) delete state.contestGrants[id]
    if (state.contestPasswords) delete state.contestPasswords[id]
  }
}

/** Five representative scoreboards plus the three original lifecycle demos. */
export function ensureContestExamples(state: MockState, now = Date.now()) {
  if (state.scoreboardExamplesVersion === 3) return false
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
  const nextContest = Math.max(0, ...state.contests.map((c) => Number(c.publicId) || 0)) + 1
  let nextSubmission = Math.max(0, ...state.submissions.map((s) => Number(s.publicId) || 0)) + 1
  let serial =
    Math.max(
      0,
      ...state.submissions
        .filter((s) => s.id.startsWith('9400'))
        .map((s) => Number(s.id.slice(4, 8))),
    ) + 1
  removeRedundantExamples(state)
  state.contestEntries ??= {}
  for (const { index, format, frozen, ended } of examples) {
    const id = mockID(9300, index)
    const offset = ended ? -240 * minute : 0
    const at = (minutes: number) => new Date(begin + offset + minutes * minute).toISOString()
    const title = `${contestFormatName(format)} 榜单演示 · ${format === 'oi' ? (ended ? '赛后公布' : '赛中不反馈') : frozen ? '封榜' : '未封榜'}`
    const description = `${entrants.length} 位选手 · ${problems.length} 道题。${contestFormatDescription[format]}${format === 'oi' ? (ended ? '本场已结束，公开榜单展示最终成绩。' : '可用裁判内部视图查看数据，选手看不到正式结果。') : frozen ? '切换公开视图查看封榜效果。' : '实时公开榜单。'}`
    const existing = state.contests.find((c) => c.id === id)
    if (existing) continue
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
      publicId: String(nextContest + index - 1),
      domainId: officialDomainID,
      title,
      description,
      ownerName: adminUser.username,
      createdAt: at(-1440),
      rule: format,
      format,
      feedback:
        format === 'oi'
          ? 'none'
          : format === 'icpc'
            ? 'summary'
            : format === 'cf'
              ? 'first_error'
              : 'full',
      medals: defaultMedals(format),
      penaltyMinutes: 20,
      penalizeCompileError: format === 'oi' || format === 'leduo',
      rankboardVisible: true,
      showProblemMetadata: false,
      submissionVisibility: format === 'oi' ? 'own' : 'during',
      sourceCodeVisibility: 'after_end',
      frozenSubmissionVisibility: 'pending',
      freezeAt: frozen ? at(60) : undefined,
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
  state.scoreboardExamplesVersion = 3
  return true
}
