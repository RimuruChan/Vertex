import { useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ListOrdered, Plus, Trash2 } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import type {
  DtoContestUpsertRequestRule as ContestRule,
  DtoContestResponseFeedback as ContestFeedback,
  DtoContestResponse as Contest,
  DtoContestUpsertRequest as ContestUpsert,
  DtoProblemResponse as Problem,
} from '@/generated/api/model'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime, fromLocalInput, toLocalInput } from '@/lib/format'

const PAGE_SIZE = 20

const visibilities = [
  { value: 'public', label: '公开' },
  { value: 'private', label: '私有' },
  { value: 'password', label: '密码赛' },
]

type ContestDraft = {
  title: string
  description: string
  rule: ContestRule
  beginAt: string
  endAt: string
  freezeAt: string
  unfreezeAt: string
  penaltyMinutes: number
  penalizeCompileError: boolean
  feedback: ContestFeedback
  visibility: string
  password: string
  rankboardVisible: boolean
}

const emptyDraft: ContestDraft = {
  title: '',
  description: '',
  rule: 'icpc',
  beginAt: '',
  endAt: '',
  freezeAt: '',
  unfreezeAt: '',
  penaltyMinutes: 20,
  penalizeCompileError: true,
  feedback: 'full',
  visibility: 'public',
  password: '',
  rankboardVisible: true,
}

/** Each format scores differently, so the editor explains the choice inline. */
const rules: { value: ContestRule; label: string; hint: string }[] = [
  {
    value: 'icpc',
    label: 'ICPC / ACM',
    hint: '按通过题数排名,同题数比罚时;只认首次通过。',
  },
  {
    value: 'ioi',
    label: 'IOI',
    hint: '按总分排名,每题取历史最高分;不计罚时。',
  },
  {
    value: 'oi',
    label: 'OI',
    hint: '按总分排名,每题只算最后一次提交;默认比赛中不公布结果。',
  },
]

const feedbacks: { value: ContestFeedback; label: string; hint: string }[] = [
  { value: 'full', label: '完整反馈', hint: '选手可以看到逐测试点结果' },
  { value: 'summary', label: '仅判定', hint: '只显示最终判定,不显示测试点明细' },
  { value: 'none', label: '不反馈', hint: '比赛中只显示「已提交」' },
]

function visibilityLabel(visibility: string): string {
  return visibilities.find((item) => item.value === visibility)?.label ?? visibility
}

export default function AdminContestPage() {
  const { can } = useDomain()
  const {
    getApiAdminContests: adminListContests,
    postApiAdminContests: adminCreateContest,
    putApiAdminContestsId: adminUpdateContest,
  } = useDomainAPI()
  const toast = useToast()
  const [contests, setContests] = useState<Contest[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)

  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<Contest | null>(null)
  const [draft, setDraft] = useState<ContestDraft>(emptyDraft)
  const [saving, setSaving] = useState(false)
  const [managing, setManaging] = useState<Contest | null>(null)

  async function load() {
    setLoading(true)
    try {
      const result = await adminListContests({ page, size: PAGE_SIZE })
      setContests(result.items)
      setTotal(result.total)
    } catch (error) {
      toast.error(apiError(error, '比赛列表加载失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page])

  function openCreate() {
    if (!can('contest.create')) return
    setEditing(null)
    setDraft(emptyDraft)
    setEditorOpen(true)
  }

  function openEdit(contest: Contest) {
    setEditing(contest)
    setDraft({
      title: contest.title,
      description: contest.description ?? '',
      rule: contest.format as ContestRule,
      beginAt: toLocalInput(contest.beginAt),
      endAt: toLocalInput(contest.endAt),
      freezeAt: toLocalInput(contest.freezeAt),
      unfreezeAt: toLocalInput(contest.unfreezeAt),
      penaltyMinutes: contest.penaltyMinutes,
      penalizeCompileError: contest.penalizeCompileError,
      feedback: contest.feedback as ContestFeedback,
      visibility: contest.visibility,
      // Never prefill the existing password; empty means "leave unchanged".
      password: '',
      rankboardVisible: contest.rankboardVisible,
    })
    setEditorOpen(true)
  }

  async function handleSave() {
    const beginAt = fromLocalInput(draft.beginAt)
    const endAt = fromLocalInput(draft.endAt)
    if (!draft.title.trim()) {
      toast.warning('请填写比赛名称')
      return
    }
    if (!beginAt || !endAt) {
      toast.warning('请选择比赛起止时间')
      return
    }
    if (new Date(endAt) <= new Date(beginAt)) {
      toast.warning('结束时间必须晚于开始时间')
      return
    }
    if (draft.visibility === 'password' && !editing && !draft.password) {
      toast.warning('密码赛需要设置比赛密码')
      return
    }

    const payload: ContestUpsert = {
      title: draft.title.trim(),
      description: draft.description,
      rule: draft.rule,
      beginAt,
      endAt,
      freezeAt: fromLocalInput(draft.freezeAt),
      unfreezeAt: fromLocalInput(draft.unfreezeAt),
      penaltyMinutes: draft.penaltyMinutes,
      penalizeCompileError: draft.penalizeCompileError,
      feedback: draft.feedback,
      visibility: draft.visibility,
      password: draft.visibility === 'password' && draft.password ? draft.password : undefined,
      rankboardVisible: draft.rankboardVisible,
    }

    setSaving(true)
    try {
      if (editing) {
        await adminUpdateContest(editing.id, payload)
        toast.success('比赛信息已更新')
        setEditorOpen(false)
        await load()
      } else {
        const created = await adminCreateContest(payload)
        toast.success('比赛已创建,接下来选择题目')
        setEditorOpen(false)
        setManaging(created)
        if (page === 1) await load()
        else setPage(1)
      }
    } catch (error) {
      toast.error(apiError(error, editing ? '更新失败' : '创建失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">比赛管理</h1>
          <p className="text-sm text-muted-foreground">共 {total} 场</p>
        </div>
        <Button onClick={openCreate} disabled={!can('contest.create')}>
          <Plus />
          创建比赛
        </Button>
      </div>

      <Card className="overflow-hidden">
        {loading ? (
          <div className="flex flex-col gap-2 p-4">
            {Array.from({ length: 5 }, (_, index) => (
              <Skeleton key={index} className="h-10 w-full" />
            ))}
          </div>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>标题</TableHead>
                  <TableHead className="w-20">赛制</TableHead>
                  <TableHead className="w-24">可见性</TableHead>
                  <TableHead className="hidden w-44 md:table-cell">开始</TableHead>
                  <TableHead className="hidden w-44 lg:table-cell">结束</TableHead>
                  <TableHead className="w-52 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {contests.length === 0 ? (
                  <TableEmpty colSpan={6}>
                    <EmptyState
                      title="还没有比赛"
                      description="创建一场比赛,然后组题。"
                      action={
                        <Button onClick={openCreate} disabled={!can('contest.create')}>
                          创建比赛
                        </Button>
                      }
                    />
                  </TableEmpty>
                ) : (
                  contests.map((contest) => (
                    <TableRow key={contest.id}>
                      <TableCell className="max-w-0 truncate font-medium">
                        {contest.title}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{contest.format.toUpperCase()}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={contest.visibility === 'public' ? 'success' : 'secondary'}>
                          {visibilityLabel(contest.visibility)}
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden text-xs text-muted-foreground md:table-cell">
                        {formatDateTime(contest.beginAt)}
                      </TableCell>
                      <TableCell className="hidden text-xs text-muted-foreground lg:table-cell">
                        {formatDateTime(contest.endAt)}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-1">
                          <Button variant="outline" size="sm" onClick={() => openEdit(contest)}>
                            编辑
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => setManaging(contest)}>
                            <ListOrdered />
                            题目
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
            <Pagination page={page} size={PAGE_SIZE} total={total} onChange={setPage} />
          </>
        )}
      </Card>

      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editing ? `编辑比赛 · ${editing.title}` : '创建比赛'}</DialogTitle>
          </DialogHeader>

          <div className="flex flex-col gap-3">
            <Field label="标题" id="contest-title">
              <Input
                id="contest-title"
                value={draft.title}
                onChange={(event) => setDraft({ ...draft, title: event.target.value })}
                placeholder="比赛名称"
              />
            </Field>
            <Field label="描述" id="contest-description">
              <Textarea
                id="contest-description"
                rows={3}
                value={draft.description}
                onChange={(event) => setDraft({ ...draft, description: event.target.value })}
                placeholder="比赛说明、规则或注意事项"
              />
            </Field>

            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="开始时间" id="contest-begin">
                <Input
                  id="contest-begin"
                  type="datetime-local"
                  value={draft.beginAt}
                  onChange={(event) => setDraft({ ...draft, beginAt: event.target.value })}
                />
              </Field>
              <Field label="结束时间" id="contest-end">
                <Input
                  id="contest-end"
                  type="datetime-local"
                  value={draft.endAt}
                  onChange={(event) => setDraft({ ...draft, endAt: event.target.value })}
                />
              </Field>
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="封榜时间(可选)" id="contest-freeze">
                <Input
                  id="contest-freeze"
                  type="datetime-local"
                  value={draft.freezeAt}
                  onChange={(event) => setDraft({ ...draft, freezeAt: event.target.value })}
                />
              </Field>
              <Field label="自动解榜时间(可选)" id="contest-unfreeze">
                <Input
                  id="contest-unfreeze"
                  type="datetime-local"
                  value={draft.unfreezeAt}
                  disabled={!draft.freezeAt}
                  onChange={(event) => setDraft({ ...draft, unfreezeAt: event.target.value })}
                />
              </Field>
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="每次未通过罚时(分钟)" id="contest-penalty">
                <Input
                  id="contest-penalty"
                  type="number"
                  min={0}
                  max={1440}
                  disabled={draft.rule !== 'icpc'}
                  value={draft.penaltyMinutes}
                  onChange={(event) =>
                    setDraft({ ...draft, penaltyMinutes: Number(event.target.value) || 0 })
                  }
                />
              </Field>
              <Field label="比赛中的判题反馈" id="contest-feedback">
                <Select
                  value={draft.feedback}
                  onValueChange={(value) =>
                    setDraft({ ...draft, feedback: value as ContestFeedback })
                  }
                >
                  <SelectTrigger id="contest-feedback">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {feedbacks.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>

            <p className="text-xs text-muted-foreground">
              {rules.find((item) => item.value === draft.rule)?.hint}
              {' · '}
              {feedbacks.find((item) => item.value === draft.feedback)?.hint}
            </p>

            {draft.rule === 'icpc' ? (
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="size-4 accent-primary"
                  checked={draft.penalizeCompileError}
                  onChange={(event) =>
                    setDraft({ ...draft, penalizeCompileError: event.target.checked })
                  }
                />
                编译错误计入罚时尝试(ICPC 正式规则如此)
              </label>
            ) : null}

            <div className="grid gap-3 sm:grid-cols-3">
              <Field label="赛制" id="contest-rule">
                <Select
                  value={draft.rule}
                  onValueChange={(value) => {
                    const rule = value as ContestRule
                    // OI is scored on the final submission, so live feedback would
                    // change what contestants can do; default it to silent.
                    setDraft({
                      ...draft,
                      rule,
                      feedback: rule === 'oi' ? 'none' : draft.feedback,
                    })
                  }}
                >
                  <SelectTrigger id="contest-rule">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {rules.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="可见性" id="contest-visibility">
                <Select
                  value={draft.visibility}
                  onValueChange={(value) => setDraft({ ...draft, visibility: value })}
                >
                  <SelectTrigger id="contest-visibility">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {visibilities.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field label="显示榜单" id="contest-rankboard">
                <label className="flex h-9 items-center gap-2 text-sm">
                  <input
                    id="contest-rankboard"
                    type="checkbox"
                    className="size-4 accent-[var(--primary)]"
                    checked={draft.rankboardVisible}
                    onChange={(event) =>
                      setDraft({ ...draft, rankboardVisible: event.target.checked })
                    }
                  />
                  参赛者可见
                </label>
              </Field>
            </div>

            {draft.visibility === 'password' ? (
              <Field
                label={editing?.visibility === 'password' ? '新比赛密码(留空则不修改)' : '比赛密码'}
                id="contest-password"
              >
                <Input
                  id="contest-password"
                  type="password"
                  autoComplete="new-password"
                  value={draft.password}
                  onChange={(event) => setDraft({ ...draft, password: event.target.value })}
                />
              </Field>
            ) : null}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setEditorOpen(false)}>
              取消
            </Button>
            <Button loading={saving} onClick={handleSave}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ContestProblemManager contest={managing} onClose={() => setManaging(null)} />
    </div>
  )
}

function Field({ label, id, children }: { label: string; id: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}

type ProblemChoice = Pick<Problem, 'id' | 'title' | 'difficulty' | 'visibility'>

/** Contest composition: pick problems, then order them A, B, C… */
function ContestProblemManager({
  contest,
  onClose,
}: {
  contest: Contest | null
  onClose: () => void
}) {
  const {
    getApiAdminContestsId: adminGetContest,
    getApiAdminProblems: adminListProblems,
    putApiAdminContestsIdProblems: adminSetContestProblems,
  } = useDomainAPI()
  const toast = useToast()
  const [choices, setChoices] = useState<ProblemChoice[]>([])
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!contest) return
    setLoading(true)
    Promise.all([adminListProblems({ page: 1, size: 100 }), adminGetContest(contest.id)])
      .then(([problemList, details]) => {
        const all: ProblemChoice[] = problemList.items.map(
          ({ id, title, difficulty, visibility }) => ({ id, title, difficulty, visibility }),
        )
        // Problems already in the contest may fall outside the first page.
        for (const problem of details.problems) {
          if (!all.some((item) => item.id === problem.problemId)) {
            all.push({
              id: problem.problemId,
              title: problem.title,
              difficulty: problem.difficulty,
              visibility: problem.visibility,
            })
          }
        }
        setChoices(all)
        setSelectedIds(details.problems.map((problem) => problem.problemId))
      })
      .catch((error) => toast.error(apiError(error, '比赛题目加载失败')))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contest?.id])

  const choiceMap = useMemo(() => new Map(choices.map((item) => [item.id, item])), [choices])
  const available = choices.filter((problem) => !selectedIds.includes(problem.id))

  function move(index: number, delta: number) {
    const target = index + delta
    if (target < 0 || target >= selectedIds.length) return
    const next = [...selectedIds]
    ;[next[index], next[target]] = [next[target], next[index]]
    setSelectedIds(next)
  }

  async function save() {
    if (!contest) return
    setSaving(true)
    try {
      await adminSetContestProblems(contest.id, { problemIds: selectedIds })
      toast.success(`已保存 ${selectedIds.length} 道比赛题目`)
      onClose()
    } catch (error) {
      toast.error(apiError(error, '比赛题目保存失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={contest !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>管理题目 · {contest?.title}</DialogTitle>
        </DialogHeader>

        {loading ? (
          <Skeleton className="h-48 w-full" />
        ) : (
          <div className="flex flex-col gap-3">
            <Select
              value=""
              onValueChange={(value) => value && setSelectedIds([...selectedIds, value])}
            >
              <SelectTrigger>
                <SelectValue
                  placeholder={available.length ? '选择要加入的题目' : '没有可加入的题目'}
                />
              </SelectTrigger>
              <SelectContent>
                {available.map((problem) => (
                  <SelectItem key={problem.id} value={problem.id}>
                    {problem.title}(难度 {problem.difficulty})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {selectedIds.length === 0 ? (
              <EmptyState title="尚未选择题目" description="从上方下拉框中加入题目。" />
            ) : (
              <ul className="flex flex-col divide-y divide-border rounded-lg border border-border">
                {selectedIds.map((problemId, index) => {
                  const problem = choiceMap.get(problemId)
                  return (
                    <li key={problemId} className="flex items-center gap-3 px-3 py-2">
                      <span className="grid size-6 shrink-0 place-items-center rounded bg-muted font-mono text-xs font-semibold">
                        {String.fromCharCode(65 + index)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium">
                          {problem?.title ?? problemId}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          难度 {problem?.difficulty ?? '—'}
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="上移"
                        disabled={index === 0}
                        onClick={() => move(index, -1)}
                      >
                        <ArrowUp />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="下移"
                        disabled={index === selectedIds.length - 1}
                        onClick={() => move(index, 1)}
                      >
                        <ArrowDown />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="移除"
                        className="hover:text-destructive"
                        onClick={() => setSelectedIds(selectedIds.filter((id) => id !== problemId))}
                      >
                        <Trash2 />
                      </Button>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button loading={saving} onClick={save}>
            保存题目顺序
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
