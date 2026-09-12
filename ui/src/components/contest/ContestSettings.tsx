import { useEffect, useRef, useState } from 'react'
import type { DtoContestResponse } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useActiveRef } from '@/domain/useActiveRef'
import { useNavigate } from '@/domain/navigation'
import { Button } from '@/components/ui/button'
import { SaveButton } from '@/components/ui/save-button'
import { Card } from '@/components/ui/card'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DateTimePicker } from '@/components/ui/date-time-picker'
import { ChoiceSelect } from '@/components/ui/choice-select'
import { shiftLocalMinutes } from '@/lib/date-time'
import { cn } from '@/lib/utils'
import { useToast } from '@/components/ui/toast'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError } from '@/lib/format'
import { contestDraft, contestPayload, type ContestDraft } from './contest-form'
import { applyContestPreset } from './contest-presets'
import { ContestPresetPicker } from './ContestPresetPicker'
import ResourceCollaboration from '@/components/ResourceCollaboration'

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
  const [saved, setSaved] = useState(false)
  const saveAnchorRef = useRef<HTMLSpanElement>(null)
  const [saveBarDocked, setSaveBarDocked] = useState(false)
  useEffect(() => {
    const anchor = saveAnchorRef.current
    if (!anchor) return
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.rootBounds) {
          setSaveBarDocked(entry.boundingClientRect.bottom <= entry.rootBounds.bottom)
        }
      },
      { rootMargin: '0px 0px -16px 0px', threshold: [0, 1] },
    )
    observer.observe(anchor)
    return () => observer.disconnect()
  }, [canEdit])
  function change<K extends keyof ContestDraft>(key: K, value: ContestDraft[K]) {
    setDraft((current) => ({ ...current, [key]: value }))
    setDirty(true)
    setSaved(false)
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
    setSaved(false)
    try {
      const result = await putApiAdminContestsId(contest.id, payload)
      if (!active.current) return
      setDraft(contestDraft(result))
      setDirty(false)
      setSaved(true)
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
  const resolvedEnd = draft.endAt,
    resolvedFreeze = draft.freezeAt
  const disabled = !canEdit || busy
  const accessDisabled = disabled || !manage
  return (
    <div className="flex flex-col gap-5">
      <form
        className="relative flex flex-col gap-5"
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <div>
          <h2 className="text-lg font-semibold">比赛设置</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {!canEdit
              ? '当前为只读。比赛进行中的修改由比赛负责人或域管理员操作。'
              : '按分组调整比赛规则，修改完成后统一保存。'}
          </p>
        </div>
        {error && (
          <p
            role="alert"
            className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"
          >
            {error}
          </p>
        )}
        <fieldset
          disabled={disabled}
          className="grid min-w-0 items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_352px]"
        >
          <div className="contents lg:flex lg:min-w-0 lg:flex-col lg:gap-6">
            <SettingsSection
              className="order-1"
              title="基本信息"
              description="向参赛者介绍这场比赛。"
            >
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
                  rows={3}
                  value={draft.description}
                  onChange={(e) => change('description', e.target.value)}
                />
              </Field>
            </SettingsSection>
            <SettingsSection
              className="order-3"
              title="计分与反馈"
              description="决定排名方式，以及选手能看到哪些评测信息。"
            >
              <ContestPresetPicker
                value={draft.rule}
                disabled={disabled}
                onApply={(preset) => {
                  setDraft((current) => applyContestPreset(current, preset))
                  setDirty(true)
                  setSaved(false)
                }}
              />
              <div className="grid gap-4 @sm:grid-cols-2">
                <div className="space-y-2 @sm:col-span-2">
                  <Label>比赛中反馈</Label>
                  <div
                    className="grid grid-cols-2 gap-2 @lg:grid-cols-4"
                    role="group"
                    aria-label="比赛中反馈"
                  >
                    {(
                      [
                        ['full', '完整反馈'],
                        ['summary', '仅最终判定'],
                        ['first_error', '首个失败测试点'],
                        ['none', '不反馈'],
                      ] as const
                    ).map(([value, label]) => (
                      <Button
                        key={value}
                        type="button"
                        variant="outline"
                        className={
                          draft.feedback === value
                            ? 'border-primary/60 bg-primary/10 text-primary hover:bg-primary/15'
                            : ''
                        }
                        aria-pressed={draft.feedback === value}
                        disabled={disabled || draft.rule === 'oi'}
                        onClick={() => change('feedback', value)}
                      >
                        {label}
                      </Button>
                    ))}
                  </div>
                  {draft.rule === 'oi' && (
                    <p className="text-xs text-muted-foreground">
                      OI 赛中不公开评测结果，结束后统一公布。
                    </p>
                  )}
                </div>
                {draft.rule === 'icpc' && (
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
                )}
              </div>
              {draft.rule === 'icpc' && (
                <ToggleRow
                  label="编译错误计入罚时尝试"
                  checked={draft.penalizeCompileError}
                  onChange={(value) => change('penalizeCompileError', value)}
                />
              )}
              <ToggleRow
                label="向选手显示榜单"
                description="关闭后仅赛务人员可查看。无反馈比赛的公开榜单在赛后开放。"
                checked={draft.rankboardVisible}
                onChange={(value) => change('rankboardVisible', value)}
              />
              <ToggleRow
                label="赛前及赛中显示难度和标签"
                description="默认隐藏，避免提示解题方向；赛后始终显示。"
                checked={draft.showProblemMetadata}
                onChange={(value) => change('showProblemMetadata', value)}
              />
            </SettingsSection>
            <SettingsSection
              className="order-4"
              title="提交可见性"
              description="分别控制记录、源码与封榜结果的公开范围。"
            >
              <div className="grid gap-4 @sm:grid-cols-2">
                <Field label="他人提交记录" id="contest-submission-visibility">
                  <ChoiceSelect
                    id="contest-submission-visibility"
                    disabled={disabled}
                    value={draft.submissionVisibility}
                    onValueChange={(value) =>
                      change('submissionVisibility', value as ContestDraft['submissionVisibility'])
                    }
                    options={[
                      ['own', '仅本人可见'],
                      ['after_end', '赛后可见'],
                      ['during', '赛中可见'],
                    ]}
                  />
                </Field>
                <Field label="他人源码" id="contest-source-visibility">
                  <ChoiceSelect
                    id="contest-source-visibility"
                    disabled={disabled}
                    value={draft.sourceCodeVisibility}
                    onValueChange={(value) =>
                      change('sourceCodeVisibility', value as ContestDraft['sourceCodeVisibility'])
                    }
                    options={[
                      ['own', '仅本人可见'],
                      ['after_end', '赛后且解榜后可见'],
                    ]}
                  />
                </Field>
                <Field label="封榜后的他人新提交" id="contest-frozen-visibility">
                  <ChoiceSelect
                    id="contest-frozen-visibility"
                    disabled={disabled}
                    value={draft.frozenSubmissionVisibility}
                    onValueChange={(value) =>
                      change(
                        'frozenSubmissionVisibility',
                        value as ContestDraft['frozenSubmissionVisibility'],
                      )
                    }
                    options={[
                      ['pending', '显示记录，结果为 Pending'],
                      ['hidden', '隐藏记录'],
                    ]}
                  />
                </Field>
              </div>
              <p className="rounded-lg bg-muted/50 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
                选手赛中不能查看他人源码。封榜时隐藏真实判定、得分和测试点详情；裁判及观察员保留赛务查看权限。
              </p>
            </SettingsSection>
          </div>
          <div className="contents lg:flex lg:min-w-0 lg:flex-col lg:gap-6">
            <SettingsSection
              className="order-2"
              title="比赛日程"
              description="设置比赛起止与榜单公布时间。"
            >
              <div className="grid items-start gap-x-6 gap-y-5 @sm:grid-cols-2">
                <ScheduleField label="开始时间" id="contest-begin">
                  <DateTimePicker
                    id="contest-begin"
                    required
                    disabled={disabled}
                    value={draft.beginAt}
                    onChange={(value) => change('beginAt', value)}
                  />
                </ScheduleField>
                <ScheduleField label="结束时间" id="contest-end">
                  <DateTimePicker
                    id="contest-end"
                    required
                    disabled={disabled}
                    min={shiftLocalMinutes(draft.beginAt, 1)}
                    value={draft.endAt}
                    onChange={(value) => change('endAt', value)}
                  />
                </ScheduleField>
                <ScheduleField label="封榜时间（可选）" id="contest-freeze">
                  <DateTimePicker
                    id="contest-freeze"
                    placeholder="不封榜"
                    disabled={disabled}
                    min={shiftLocalMinutes(draft.beginAt, 1)}
                    max={shiftLocalMinutes(resolvedEnd, -1)}
                    value={draft.freezeAt}
                    onChange={(value) => {
                      change('freezeAt', value)
                      if (!value) change('unfreezeAt', '')
                    }}
                  />
                </ScheduleField>
                <ScheduleField label="解榜方式" id="contest-unfreeze-mode">
                  <ChoiceSelect
                    id="contest-unfreeze-mode"
                    disabled={disabled || !resolvedFreeze}
                    value={
                      !resolvedFreeze
                        ? 'off'
                        : draft.unfreezeAt === 'now'
                          ? 'now'
                          : draft.unfreezeAt === 'end'
                            ? 'end'
                            : draft.unfreezeAt
                              ? 'scheduled'
                              : 'manual'
                    }
                    onValueChange={(value) =>
                      change(
                        'unfreezeAt',
                        value === 'manual' ? '' : value === 'scheduled' ? resolvedEnd : value,
                      )
                    }
                    options={[
                      ...(!resolvedFreeze ? [['off', '未启用封榜'] as const] : []),
                      ['manual', '保持封榜，手动公布'],
                      ['end', '比赛结束时公布'],
                      ['scheduled', '指定公布时间'],
                      ...(resolvedFreeze && Date.parse(resolvedFreeze) <= Date.now()
                        ? [['now', '立即解榜'] as const]
                        : []),
                    ]}
                  />
                  {draft.unfreezeAt && !['now', 'end'].includes(draft.unfreezeAt) && (
                    <DateTimePicker
                      id="contest-unfreeze"
                      disabled={disabled || !resolvedFreeze}
                      min={resolvedFreeze}
                      value={draft.unfreezeAt}
                      onChange={(value) => change('unfreezeAt', value)}
                    />
                  )}
                  <p className="text-xs leading-5 text-muted-foreground">
                    {!resolvedFreeze
                      ? ''
                      : draft.unfreezeAt === 'now'
                        ? '保存设置后立即公布榜单。'
                        : draft.unfreezeAt === 'end'
                          ? `将在 ${resolvedEnd.replace('T', ' ')} 公布。`
                          : '封榜期间，选手无法查看最终排名。'}
                  </p>
                </ScheduleField>
              </div>
            </SettingsSection>
            <SettingsSection
              className="order-5"
              title="访问与报名"
              description="设置谁能进入比赛，以及如何获得参赛资格。"
            >
              {!manage && (
                <p className="text-xs text-muted-foreground">
                  此分组仅比赛负责人或域管理员可修改。
                </p>
              )}
              <fieldset disabled={accessDisabled} className="flex flex-col gap-4">
                <div className="grid gap-4 @sm:grid-cols-2">
                  <Field label="可见性" id="contest-visibility">
                    <ChoiceSelect
                      id="contest-visibility"
                      disabled={accessDisabled}
                      value={draft.visibility}
                      onValueChange={(value) => change('visibility', value)}
                      options={[
                        ['private', '私有'],
                        ['public', '公开'],
                        ['password', '密码赛'],
                      ]}
                    />
                  </Field>
                  <Field label="参赛资格" id="contest-admission">
                    <ChoiceSelect
                      id="contest-admission"
                      disabled={accessDisabled}
                      value={draft.admission}
                      onValueChange={(value) =>
                        change('admission', value as ContestDraft['admission'])
                      }
                      options={[
                        ['members', '域内可提交的成员'],
                        ['restricted', '指定用户或群组'],
                      ]}
                    />
                  </Field>
                  {draft.visibility === 'password' && (
                    <Field label="比赛密码" id="contest-password">
                      <Input
                        id="contest-password"
                        type="password"
                        autoComplete="new-password"
                        value={draft.password}
                        placeholder={
                          contest.visibility === 'password' ? '留空保留现有密码' : '设置入场密码'
                        }
                        onChange={(e) => change('password', e.target.value)}
                      />
                    </Field>
                  )}
                </div>
                {manage && draft.admission === 'restricted' && (
                  <ResourceCollaboration
                    kind="contest"
                    id={contest.id}
                    ownerId={contest.ownerId}
                    ownerName={contest.ownerName}
                    manage={!accessDisabled}
                    transfer={false}
                    onChanged={onSaved}
                    participantsOnly
                  />
                )}
                <ToggleRow
                  label="允许自助报名"
                  description="符合参赛资格的用户可以自行报名。关闭不会取消已有报名。"
                  checked={draft.allowSelfRegistration}
                  onChange={(value) => change('allowSelfRegistration', value)}
                />
                <ToggleRow
                  label="允许开赛后报名"
                  description="比赛计时不会因迟到报名延长，结束后始终关闭报名。"
                  disabled={!draft.allowSelfRegistration}
                  checked={draft.allowLateRegistration}
                  onChange={(value) => change('allowLateRegistration', value)}
                />
              </fieldset>
            </SettingsSection>
          </div>
        </fieldset>
        {canEdit && (
          <div
            className={`sticky bottom-4 z-20 mx-auto flex max-w-full flex-wrap items-center gap-y-2 border border-border py-2.5 ${
              saveBarDocked
                ? 'w-full justify-between gap-x-3 rounded-xl bg-card px-5'
                : 'w-fit justify-center gap-x-6 rounded-2xl bg-card/95 px-4 shadow-lg backdrop-blur-sm'
            }`}
          >
            <span className="text-sm text-muted-foreground" role="status">
              {dirty ? '有未保存修改' : '设置已同步'}
            </span>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="ghost"
                disabled={!dirty || busy}
                onClick={() => {
                  setDraft(contestDraft(contest))
                  setDirty(false)
                  setError(null)
                }}
              >
                撤销修改
              </Button>
              <SaveButton type="submit" loading={busy} saved={saved} disabled={!dirty}>
                保存设置
              </SaveButton>
            </div>
          </div>
        )}
        {canEdit && (
          <span
            ref={saveAnchorRef}
            aria-hidden="true"
            className="pointer-events-none absolute bottom-0 h-px w-px"
          />
        )}
      </form>
      {contest.permissions.delete && (
        <Card className="mt-2 flex flex-wrap items-center justify-between gap-3 rounded-xl border-destructive/25 p-5">
          <div>
            <h2 className="text-sm font-medium">删除比赛</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              有参赛记录时保留历史，不执行级联删除。
            </p>
          </div>
          <Button
            variant="outline"
            className="text-destructive hover:bg-destructive/10"
            disabled={busy}
            onClick={() => void remove()}
          >
            删除比赛
          </Button>
        </Card>
      )}
    </div>
  )
}

function SettingsSection({
  title,
  description,
  children,
  className,
}: {
  title: string
  description: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <Card className={cn('@container flex min-w-0 flex-col gap-5 rounded-xl p-5 sm:p-6', className)}>
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        <p className="mt-2 text-xs leading-relaxed text-muted-foreground">{description}</p>
      </div>
      <div className="flex min-w-0 flex-col gap-4">{children}</div>
    </Card>
  )
}
function ToggleRow({
  label,
  description,
  checked,
  onChange,
  disabled,
}: {
  label: string
  description?: string
  checked: boolean
  onChange: (value: boolean) => void
  disabled?: boolean
}) {
  return (
    <label className="flex items-start gap-3 rounded-lg border border-border/70 px-3 py-3 text-sm has-[:disabled]:opacity-60">
      <input
        type="checkbox"
        className="mt-0.5 size-4 shrink-0"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>
        <span className="font-medium">{label}</span>
        {description && (
          <span className="mt-1 block text-xs leading-relaxed text-muted-foreground">
            {description}
          </span>
        )}
      </span>
    </label>
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

function ScheduleField({
  label,
  id,
  children,
}: {
  label: string
  id: string
  children: React.ReactNode
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id} className="text-sm">
        {label.replace('（可选）', '')}
      </Label>
      <div className="min-w-0 space-y-2">{children}</div>
    </div>
  )
}
