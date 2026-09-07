import type {
  DtoContestResponse,
  DtoContestUpsertRequest,
  DtoContestProblemResponse,
} from '@/generated/api/model'
import { fromLocalInput, toLocalInput } from '@/lib/format'

export type ContestDraft = {
  title: string
  description: string
  rule: DtoContestResponse['format']
  beginAt: string
  endAt: string
  freezeAt: string
  unfreezeAt: string
  penaltyMinutes: number
  penalizeCompileError: boolean
  feedback: DtoContestResponse['feedback']
  visibility: string
  admission: DtoContestResponse['admission']
  allowSelfRegistration: boolean
  allowLateRegistration: boolean
  password: string
  rankboardVisible: boolean
}

export function newContestDraft(now = Date.now()): ContestDraft {
  return {
    title: '',
    description: '',
    rule: 'icpc',
    beginAt: toLocalInput(new Date(now + 86400000).toISOString()),
    endAt: toLocalInput(new Date(now + 104400000).toISOString()),
    freezeAt: '',
    unfreezeAt: '',
    penaltyMinutes: 20,
    penalizeCompileError: true,
    feedback: 'full',
    visibility: 'private',
    admission: 'members',
    allowSelfRegistration: true,
    allowLateRegistration: false,
    password: '',
    rankboardVisible: true,
  }
}

export function contestDraft(contest: DtoContestResponse): ContestDraft {
  return {
    ...contest,
    rule: contest.format,
    password: '',
    beginAt: toLocalInput(contest.beginAt),
    endAt: toLocalInput(contest.endAt),
    freezeAt: toLocalInput(contest.freezeAt),
    unfreezeAt: toLocalInput(contest.unfreezeAt),
  }
}

export function contestPayload(
  draft: ContestDraft,
  previousVisibility?: string,
): DtoContestUpsertRequest {
  const beginAt = fromLocalInput(draft.beginAt),
    endAt = fromLocalInput(draft.endAt),
    freezeAt = fromLocalInput(draft.freezeAt),
    unfreezeAt = fromLocalInput(draft.unfreezeAt)
  if (!draft.title.trim()) throw new Error('请填写比赛名称')
  if (!beginAt || !endAt || endAt <= beginAt)
    throw new Error('请选择正确的起止时间，结束须晚于开始')
  if (draft.freezeAt && (!freezeAt || freezeAt <= beginAt || freezeAt >= endAt))
    throw new Error('封榜时间须在比赛起止之间')
  if (draft.unfreezeAt && (!unfreezeAt || !freezeAt || unfreezeAt < freezeAt))
    throw new Error('解榜须设置封榜时间，且不能早于封榜')
  if (
    !Number.isInteger(draft.penaltyMinutes) ||
    draft.penaltyMinutes < 0 ||
    draft.penaltyMinutes > 1440
  )
    throw new Error('罚时须为 0–1440 的整数')
  if (draft.visibility === 'password' && previousVisibility !== 'password' && !draft.password)
    throw new Error('请设置比赛密码')
  // Select known request fields; a response also contains IDs, owner and capabilities.
  return {
    title: draft.title.trim(),
    description: draft.description,
    rule: draft.rule,
    beginAt,
    endAt,
    freezeAt,
    unfreezeAt,
    penaltyMinutes: draft.penaltyMinutes,
    penalizeCompileError: draft.penalizeCompileError,
    feedback: draft.feedback,
    visibility: draft.visibility,
    admission: draft.admission,
    allowSelfRegistration: draft.allowSelfRegistration,
    allowLateRegistration: draft.allowLateRegistration,
    password: draft.visibility === 'password' && draft.password ? draft.password : undefined,
    rankboardVisible: draft.rankboardVisible,
  }
}

export function canPrepareContest(contest: DtoContestResponse, now = Date.now()) {
  return (
    contest.permissions.edit &&
    (contest.permissions.manageAccess || now < Date.parse(contest.beginAt))
  )
}

export function nextProblemLabel(existing: string[]): string {
  for (let index = 0; ; index++) {
    let label = '',
      value = index
    do {
      label = String.fromCharCode(65 + (value % 26)) + label
      value = Math.floor(value / 26) - 1
    } while (value >= 0)
    if (!existing.includes(label)) return label
  }
}

export function compositionPayload(entries: DtoContestProblemResponse[]) {
  const ids = new Set<string>(),
    labels = new Set<string>()
  return {
    problems: entries.map((entry) => {
      const label = entry.label.trim(),
        color = entry.color.trim()
      if (!/^[A-Za-z][A-Za-z0-9]{0,7}$/.test(label))
        throw new Error('题号须以字母开头，最多 8 位字母或数字')
      if (ids.has(entry.problemId) || labels.has(label)) throw new Error('题目与题号不能重复')
      if (!Number.isInteger(entry.points) || entry.points < 1 || entry.points > 100000)
        throw new Error('分值须为 1–100000 的整数')
      if (color.length > 32) throw new Error('颜色最多 32 字符')
      ids.add(entry.problemId)
      labels.add(label)
      return { problemId: entry.problemId, label, color, points: entry.points }
    }),
  }
}
