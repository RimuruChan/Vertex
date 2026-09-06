import type { DtoContestPermissions, DtoRejudgingChangeResponse } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'

export function rejudgingRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
  caps: (contestId: string) => DtoContestPermissions,
  verdict: string,
) {
  const [, , , id, action] = path.split('/').filter(Boolean)
  const get = method === 'GET'
  const write = (contestId: string) => {
    if (!caps(contestId).rejudge) throw new MockError(403, '没有本场比赛的重测权限。')
  }
  const visible = (contestId?: string) =>
    !!contestId &&
    state.contests.some((contest) => contest.id === contestId) &&
    caps(contestId).viewJury
  for (const batch of state.rejudgeBatches) {
    if (batch.record.state !== 'running') continue
    batch.record.done = batch.members.filter(
      (m) => state.submissionGenerations[m.prior.id] !== m.generation || !state.pending[m.prior.id],
    ).length
    batch.record.changed = batch.members.filter((m) => {
      const current = state.submissions.find((s) => s.id === m.prior.id)
      return (
        current &&
        state.submissionGenerations[m.prior.id] === m.generation &&
        !state.pending[m.prior.id] &&
        (current.status !== m.prior.status || current.score !== m.prior.score)
      )
    }).length
    if (batch.record.done === batch.record.total) {
      batch.record.state = 'finished'
      batch.record.finishedAt = new Date(now).toISOString()
    }
  }
  if (!id && get) {
    const items = state.rejudgeBatches.filter(
      (b) =>
        visible(b.record.contestId) && (!params.contest || b.record.contestId === params.contest),
    )
    return {
      items: items.slice(0, Math.min(Number(params.limit) || 20, 100)).map((b) => b.record),
      total: items.length,
    }
  }
  if (!id && method === 'POST') {
    const contestId = typeof body.contestId === 'string' ? body.contestId : ''
    if (!contestId) throw new MockError(400, '比赛重测必须指定比赛。')
    write(contestId)
    const matched = state.submissions
      .filter(
        (s) =>
          s.contestId === contestId &&
          !state.pending[s.id] &&
          s.status !== 'Pending' &&
          s.status !== 'Judging' &&
          (!body.problemId || body.problemId === s.problemId) &&
          (!body.status || body.status === s.status) &&
          (!body.language || body.language === s.language) &&
          (!body.userId || body.userId === s.userId) &&
          (!Array.isArray(body.submissionIds) || body.submissionIds.includes(s.id)),
      )
      .slice(0, 5000)
    if (!matched.length) throw new MockError(400, '筛选条件没有匹配到已完成提交。')
    const batch: MockState['rejudgeBatches'][number] = {
      record: {
        id: crypto.randomUUID(),
        contestId,
        problemId: typeof body.problemId === 'string' ? body.problemId : undefined,
        reason: String(body.reason ?? ''),
        createdAt: new Date(now).toISOString(),
        state: 'running',
        total: matched.length,
        done: 0,
        changed: 0,
      },
      members: matched.map((s) => ({
        prior: structuredClone(s),
        generation: (state.submissionGenerations[s.id] ?? 1) + 1,
        started: now,
      })),
    }
    for (const member of batch.members) {
      const current = state.submissions.find((s) => s.id === member.prior.id)!
      state.submissionGenerations[current.id] = member.generation
      state.pending[current.id] = { started: now, verdict }
      Object.assign(current, {
        status: 'Pending',
        judgedAt: undefined,
        judgedCases: 0,
        totalCases: 0,
        caseResults: [],
        compileResult: '',
        score: 0,
        totalTimeMs: 0,
        peakMemoryKb: 0,
      })
    }
    state.rejudgeBatches.unshift(batch)
    return batch.record
  }
  const batch = state.rejudgeBatches.find((b) => b.record.id === id)
  if (!batch || !visible(batch.record.contestId)) throw new MockError(404, '重测记录不存在。')
  if (get && !action) return batch.record
  if (get && action === 'changes') {
    const items: DtoRejudgingChangeResponse[] = batch.members.flatMap((m) => {
      const current = state.submissions.find((s) => s.id === m.prior.id)
      if (!current || (current.status === m.prior.status && current.score === m.prior.score))
        return []
      return [
        {
          submissionId: current.id,
          username: current.username ?? '',
          problemTitle: current.problemTitle ?? '',
          priorStatus: m.prior.status,
          priorScore: m.prior.score,
          status: current.status,
          score: current.score,
          judged:
            !state.pending[current.id] && state.submissionGenerations[current.id] === m.generation,
        },
      ]
    })
    return { items, total: items.length }
  }
  if (method === 'POST' && action === 'cancel') {
    write(batch.record.contestId!)
    if (batch.record.state !== 'running') throw new MockError(400, '重测批次已经结束。')
    for (const m of batch.members) {
      const current = state.submissions.find((s) => s.id === m.prior.id)
      if (
        current &&
        now - m.started < 900 &&
        state.submissionGenerations[current.id] === m.generation
      ) {
        Object.assign(current, structuredClone(m.prior))
        delete state.pending[current.id]
      }
    }
    batch.record.state = 'cancelled'
    batch.record.finishedAt = new Date(now).toISOString()
    return { status: 'cancelled' }
  }
  throw new MockError(501, '此重测接口尚未提供 mock，未向真实后端发送请求。')
}
