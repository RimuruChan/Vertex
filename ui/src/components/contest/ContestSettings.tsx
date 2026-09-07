import { useState } from 'react'
import type { DtoContestResponse } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useActiveRef } from '@/domain/useActiveRef'
import { useNavigate } from '@/domain/navigation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/components/ui/toast'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError } from '@/lib/format'
import { contestDraft, contestPayload, type ContestDraft } from './contest-form'

export default function ContestSettings({
  contest,
  canEdit,
  onSaved,
}: {
  contest: DtoContestResponse
  canEdit: boolean
  onSaved: () => void
}) {
  const { putApiAdminContestsId, deleteApiContestsId } = useDomainAPI()
  const [draft, setDraft] = useState(() => contestDraft(contest)),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null),
    [dirty, setDirty] = useState(false)
  const active = useActiveRef(),
    toast = useToast(),
    confirm = useConfirm(),
    navigate = useNavigate()
  const manage = contest.permissions.manageAccess
  function change<K extends keyof ContestDraft>(key: K, value: ContestDraft[K]) {
    setDraft((current) => ({ ...current, [key]: value }))
    setDirty(true)
  }
  async function save() {
    if (!canEdit || busy) return
    setError(null)
    let payload
    try {
      payload = contestPayload(draft, contest.visibility)
    } catch (cause) {
      setError((cause as Error).message)
      return
    }
    setBusy(true)
    try {
      const result = await putApiAdminContestsId(contest.id, payload)
      if (!active.current) return
      setDraft(contestDraft(result))
      setDirty(false)
      toast.success('比赛设置已保存')
      onSaved()
    } catch (cause) {
      if (active.current) setError(apiError(cause, '比赛设置保存失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  async function remove() {
    if (
      !contest.permissions.delete ||
      busy ||
      !(await confirm({
        title: `删除「${contest.title}」？`,
        description:
          '仅可删除没有报名、提交或澄清记录的比赛。删除无法恢复；有历史记录时请调整可见性。',
        confirmLabel: '删除比赛',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await deleteApiContestsId(contest.id)
      if (!active.current) return
      toast.success('比赛已删除')
      navigate('/manage/contests')
    } catch (cause) {
      if (active.current) setError(apiError(cause, '删除比赛失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <Card className="p-4 sm:p-5">
        <form
          className="space-y-5"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
        >
          <div>
            <h2 className="font-medium">比赛设置</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {!canEdit
                ? '只读设置。编辑协作者只可在开赛前准备比赛；进行中调整需要 owner 或域资源管理者。'
                : '设置与题目编排分别保存，切换分区不会丢失本页未保存的修改。'}
            </p>
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <fieldset disabled={!canEdit || busy} className="space-y-4">
            <Field label="比赛名称" id="contest-title">
              <Input
                id="contest-title"
                required
                value={draft.title}
                onChange={(e) => change('title', e.target.value)}
              />
            </Field>
            <Field label="比赛说明" id="contest-description">
              <Textarea
                id="contest-description"
                rows={4}
                value={draft.description}
                onChange={(e) => change('description', e.target.value)}
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="开始时间" id="contest-begin">
                <Input
                  id="contest-begin"
                  type="datetime-local"
                  required
                  value={draft.beginAt}
                  onChange={(e) => change('beginAt', e.target.value)}
                />
              </Field>
              <Field label="结束时间" id="contest-end">
                <Input
                  id="contest-end"
                  type="datetime-local"
                  required
                  value={draft.endAt}
                  onChange={(e) => change('endAt', e.target.value)}
                />
              </Field>
              <Field label="封榜时间（可选）" id="contest-freeze">
                <Input
                  id="contest-freeze"
                  type="datetime-local"
                  value={draft.freezeAt}
                  onChange={(e) => {
                    change('freezeAt', e.target.value)
                    if (!e.target.value) change('unfreezeAt', '')
                  }}
                />
              </Field>
              <Field label="解榜时间（可选）" id="contest-unfreeze">
                <Input
                  id="contest-unfreeze"
                  type="datetime-local"
                  disabled={!draft.freezeAt}
                  value={draft.unfreezeAt}
                  onChange={(e) => change('unfreezeAt', e.target.value)}
                />
              </Field>
              <Field label="赛制" id="contest-rule">
                <select
                  id="contest-rule"
                  className="h-9 rounded-md border bg-background px-2"
                  value={draft.rule}
                  onChange={(e) => {
                    change('rule', e.target.value as ContestDraft['rule'])
                    if (e.target.value === 'oi') change('feedback', 'none')
                  }}
                >
                  <option value="icpc">ICPC · 通过题数与罚时</option>
                  <option value="ioi">IOI · 每题最高分</option>
                  <option value="oi">OI · 每题最后一次提交</option>
                </select>
              </Field>
              <Field label="比赛中反馈" id="contest-feedback">
                <select
                  id="contest-feedback"
                  className="h-9 rounded-md border bg-background px-2"
                  value={draft.feedback}
                  onChange={(e) => change('feedback', e.target.value as ContestDraft['feedback'])}
                >
                  <option value="full">完整反馈</option>
                  <option value="summary">仅最终判定</option>
                  <option value="none">不反馈</option>
                </select>
              </Field>
            </div>
            {draft.rule === 'icpc' && (
              <div className="grid items-end gap-4 sm:grid-cols-2">
                <Field label="每次未通过罚时（分钟）" id="contest-penalty">
                  <Input
                    id="contest-penalty"
                    type="number"
                    min={0}
                    max={1440}
                    value={draft.penaltyMinutes}
                    onChange={(e) => change('penaltyMinutes', Number(e.target.value))}
                  />
                  <p className="text-xs text-muted-foreground">0 使用默认值 20 分钟。</p>
                </Field>
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={draft.penalizeCompileError}
                    onChange={(e) => change('penalizeCompileError', e.target.checked)}
                  />
                  编译错误计入罚时尝试
                </label>
              </div>
            )}
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={draft.rankboardVisible}
                onChange={(e) => change('rankboardVisible', e.target.checked)}
              />
              向选手显示榜单
            </label>
            <fieldset disabled={!manage} className="grid gap-4 border-t pt-4 sm:grid-cols-2">
              <Field label="可见性" id="contest-visibility">
                <select
                  id="contest-visibility"
                  className="h-9 rounded-md border bg-background px-2"
                  value={draft.visibility}
                  onChange={(e) => change('visibility', e.target.value)}
                >
                  <option value="private">私有</option>
                  <option value="public">公开</option>
                  <option value="password">密码赛</option>
                </select>
              </Field>
              <Field label="参赛资格" id="contest-admission">
                <select
                  id="contest-admission"
                  className="h-9 rounded-md border bg-background px-2"
                  value={draft.admission}
                  onChange={(e) => change('admission', e.target.value as ContestDraft['admission'])}
                >
                  <option value="members">域内可提交的成员</option>
                  <option value="restricted">协作权限中指定的用户 / group</option>
                </select>
              </Field>
              {draft.visibility === 'password' && (
                <Field
                  label={contest.visibility === 'password' ? '新密码（留空不修改）' : '比赛密码'}
                  id="contest-password"
                >
                  <Input
                    id="contest-password"
                    type="password"
                    autoComplete="new-password"
                    value={draft.password}
                    onChange={(e) => change('password', e.target.value)}
                  />
                </Field>
              )}
            </fieldset>
            <p className="text-xs text-muted-foreground">
              可见性、参赛资格和密码仅由 owner / 域资源管理者修改。参赛资格不等于已经报名。
            </p>
          </fieldset>
          {canEdit && (
            <div className="flex flex-wrap items-center gap-3">
              <Button type="submit" loading={busy} disabled={!dirty}>
                保存设置
              </Button>
              <span className="text-xs text-muted-foreground">
                {dirty ? '有未保存修改' : '设置已同步'}
              </span>
            </div>
          )}
        </form>
      </Card>
      {contest.permissions.delete && (
        <Card className="flex flex-wrap items-center justify-between gap-3 border-destructive/30 p-5">
          <div>
            <h2 className="text-sm font-medium">删除比赛</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              有参赛记录时保留历史，不执行级联删除。
            </p>
          </div>
          <Button variant="destructive" disabled={busy} onClick={() => void remove()}>
            删除比赛
          </Button>
        </Card>
      )}
    </div>
  )
}

function Field({ label, id, children }: { label: string; id: string; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}
