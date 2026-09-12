import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import { ArrowDown, ArrowUp, Lock, Pencil, Plus, Save, Trash2 } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoProblemResponse as Problem,
  DtoSetItemResponse as SetItem,
  DtoSetResponse as ProblemSet,
  DtoSetUpsertRequestVisibility as SetVisibility,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { useCanonicalResourcePath } from '@/hooks/useCanonicalPath'
import MdRenderer from '@/components/MdRenderer'
import SetCollaboration from '@/components/problemset/SetCollaboration'
import ProblemStatusIcon from '@/components/ProblemStatusIcon'
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
import { EmptyState, PageSpinner, Progress } from '@/components/ui/misc'
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
import { apiError, formatRatio } from '@/lib/format'

/** 题单详情: the curated list with the viewer's per-problem status. */
export default function ProblemSetDetailPage() {
  const {
    deleteApiProblemSetsId: deleteSet,
    getApiProblemSetsId: getSet,
    getApiProblems: listProblems,
    putApiProblemSetsId: updateSet,
    putApiProblemSetsIdItems: saveItems,
  } = useDomainAPI()
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const toast = useToast()
  const { user, ready } = useAuth()
  const requestKey = `${id}:${user?.id ?? 'anonymous'}:${user?.role ?? ''}`

  const activeKey = useRef(requestKey)
  activeKey.current = requestKey

  const [set, setSet] = useState<ProblemSet | null>(null)
  useCanonicalResourcePath('problem-sets', id, set)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [loadedKey, setLoadedKey] = useState('')
  const [reloadToken, setReloadToken] = useState(0)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [deletePending, setDeletePending] = useState(false)

  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [visibility, setVisibility] = useState<SetVisibility>('public')
  const [items, setItems] = useState<SetItem[]>([])

  const [picker, setPicker] = useState(false)
  const [candidates, setCandidates] = useState<Problem[]>([])
  const [search, setSearch] = useState('')
  const [pickerLoading, setPickerLoading] = useState(false)
  const [pickerError, setPickerError] = useState<string | null>(null)
  const [pickerPage, setPickerPage] = useState(1),
    [pickerTotal, setPickerTotal] = useState(0)
  const pickerController = useRef<AbortController | null>(null)

  const hydrate = useCallback((result: ProblemSet) => {
    setSet(result)
    setTitle(result.title)
    setDescription(result.description)
    setVisibility(result.visibility as SetVisibility)
    setItems(result.items)
  }, [])

  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    setNotFound(false)
    setSet(null)
    setEditing(false)
    setSaving(false)
    setDeleting(false)
    setDeletePending(false)
    pickerController.current?.abort()
    setPicker(false)
    setCandidates([])
    setPickerLoading(false)
    setPickerError(null)
    getSet(id, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) return
        hydrate(result)
        setLoadedKey(requestKey)
      })
      .catch((error) => {
        if (controller.signal.aborted) return
        setSet(null)
        if ((error as { response?: { status?: number } })?.response?.status === 404) {
          setNotFound(true)
          setLoadedKey(requestKey)
          return
        }
        setLoadError(apiError(error, '题单加载失败，请稍后重试'))
        setLoadedKey(requestKey)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [hydrate, id, ready, reloadToken, requestKey, user?.id, user?.role])

  useEffect(() => () => pickerController.current?.abort(), [])

  async function openPicker() {
    await loadCandidates(1)
  }

  async function loadCandidates(page: number) {
    setPicker(true)
    pickerController.current?.abort()
    const controller = new AbortController()
    pickerController.current = controller
    setPickerLoading(true)
    setPickerError(null)
    try {
      const result = await listProblems(
        { page, size: 20, keyword: search.trim() || undefined, view: 'available' },
        { signal: controller.signal },
      )
      if (controller.signal.aborted) return
      setCandidates(result.items)
      setPickerPage(page)
      setPickerTotal(result.total)
    } catch (error) {
      if (controller.signal.aborted) return
      setCandidates([])
      setPickerError(apiError(error, '题目加载失败，请稍后重试'))
    } finally {
      if (!controller.signal.aborted) setPickerLoading(false)
    }
  }

  function addProblem(problem: Problem) {
    if (items.some((entry) => entry.problemId === problem.id)) {
      toast.warning('该题目已在题单中')
      return
    }
    setItems((current) => [
      ...current,
      {
        problemId: problem.id,
        problemPublicId: problem.publicId,
        sortOrder: current.length,
        note: '',
        title: problem.title,
        difficulty: problem.difficulty,
        visibility: problem.visibility,
        tags: problem.tags ?? [],
        submitCount: problem.submissionCount,
        acceptCount: problem.acceptedCount,
        userStatus: problem.userStatus,
      },
    ])
  }

  function move(index: number, delta: number) {
    const target = index + delta
    if (target < 0 || target >= items.length) return
    setItems((current) => {
      const next = [...current]
      const [moved] = next.splice(index, 1)
      next.splice(target, 0, moved)
      return next
    })
  }

  async function handleSave() {
    if (!title.trim()) {
      toast.warning('请填写题单标题')
      return
    }
    if (saving || !set?.permissions.edit) return
    const key = requestKey
    setSaving(true)
    try {
      const updated = await updateSet(id, { title: title.trim(), description, visibility })
      if (activeKey.current !== key) return
      setSet(updated)
      toast.success('标题、简介和可见性已保存；题目编排单独保存')
    } catch (error) {
      if (activeKey.current === key) toast.error(apiError(error, '设置保存失败'))
    } finally {
      if (activeKey.current === key) setSaving(false)
    }
  }

  async function handleSaveItems() {
    if (saving || !set?.permissions.editItems) return
    const key = requestKey
    setSaving(true)
    try {
      const updated = await saveItems(id, {
        items: items.map((entry) => ({ problemId: entry.problemId, note: entry.note })),
      })
      if (activeKey.current !== key) return
      setSet(updated)
      setItems(updated.items)
      toast.success('题目编排已保存')
    } catch (error) {
      if (activeKey.current === key) toast.error(apiError(error, '题目编排保存失败'))
    } finally {
      if (activeKey.current === key) setSaving(false)
    }
  }

  async function handleDelete() {
    if (!set?.permissions.delete || deletePending) return
    const key = requestKey
    setDeletePending(true)
    try {
      await deleteSet(id)
      if (activeKey.current !== key) return
      toast.success('题单已删除')
      navigate('/problem-sets')
    } catch (error) {
      if (activeKey.current === key) toast.error(apiError(error, '删除失败'))
    } finally {
      if (activeKey.current === key) setDeletePending(false)
    }
  }

  function cancelEditing() {
    if (!set) return
    setTitle(set.title)
    setDescription(set.description)
    setVisibility(set.visibility as SetVisibility)
    setItems(set.items)
    setEditing(false)
    setPicker(false)
  }

  function updateNote(index: number, note: string) {
    setItems((current) =>
      current.map((row, position) => (position === index ? { ...row, note } : row)),
    )
  }

  if (!ready || loading || loadedKey !== requestKey) return <PageSpinner />
  if (loadError) {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16">
        <EmptyState
          title="题单加载失败"
          description={loadError}
          action={
            <div className="flex flex-wrap justify-center gap-2">
              <Button variant="outline" onClick={() => setReloadToken((value) => value + 1)}>
                重新加载
              </Button>
              <Button variant="ghost" asChild>
                <Link to="/problem-sets">返回题单列表</Link>
              </Button>
            </div>
          }
        />
      </div>
    )
  }
  if (!set) {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16">
        <EmptyState
          title={notFound ? '题单不存在' : '无法显示题单'}
          description="它可能已被删除，或者是一个你无权查看的私有题单。"
          action={
            <Button variant="outline" asChild>
              <Link to="/problem-sets">返回题单列表</Link>
            </Button>
          }
        />
      </div>
    )
  }

  const percent = set.problemCount > 0 ? (set.solvedCount / set.problemCount) * 100 : 0

  return (
    <div className="site-container flex flex-col gap-4 py-6">
      <Card className="flex flex-col gap-3 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex flex-col gap-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-xl font-semibold tracking-tight">{set.title}</h1>
              {set.visibility === 'private' ? (
                <Badge variant="secondary">
                  <Lock className="size-3" />
                  私有
                </Badge>
              ) : null}
            </div>
            <p className="text-sm text-muted-foreground">
              由 {set.ownerName} 维护 · {set.problemCount} 题
            </p>
          </div>
          {set.permissions.edit ? (
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={saving}
                onClick={() => {
                  if (editing) cancelEditing()
                  else setEditing(true)
                }}
              >
                <Pencil />
                {editing ? '取消编辑' : '编辑'}
              </Button>
              {set.permissions.delete && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="hover:text-destructive"
                  onClick={() => setDeleting(true)}
                >
                  <Trash2 />
                  删除
                </Button>
              )}
            </div>
          ) : null}
        </div>

        {editing ? (
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-title">标题</Label>
              <Input id="edit-title" value={title} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-description">简介(支持 Markdown)</Label>
              <Textarea
                id="edit-description"
                rows={5}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
            <div className="flex flex-wrap items-end gap-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="edit-visibility">可见性</Label>
                <Select
                  disabled={!set.permissions.publish || saving}
                  value={visibility}
                  onValueChange={(value) => setVisibility(value as SetVisibility)}
                >
                  <SelectTrigger id="edit-visibility" className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="public">公开</SelectItem>
                    <SelectItem value="private">私有</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button
                variant="outline"
                disabled={!set.permissions.editItems || saving}
                onClick={openPicker}
              >
                <Plus />
                添加题目
              </Button>
              <Button disabled={!title.trim()} loading={saving} onClick={handleSave}>
                <Save />
                保存设置
              </Button>
            </div>
          </div>
        ) : (
          <>
            {set.description ? <MdRenderer content={set.description} /> : null}
            {user ? (
              <div className="flex flex-col gap-1.5">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">完成进度</span>
                  <span className="tabular-nums">
                    {set.solvedCount} / {set.problemCount}
                  </span>
                </div>
                <Progress value={percent} />
              </div>
            ) : null}
          </>
        )}
      </Card>

      {set.permissions.viewAccess && !editing && (
        <SetCollaboration
          key={requestKey}
          set={set}
          onTransferred={() => setReloadToken((v) => v + 1)}
        />
      )}

      <Card className="overflow-hidden">
        {editing && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
            <p className="text-sm text-muted-foreground">
              {set.permissions.editItems
                ? '题目顺序和备注与上方设置分别保存。'
                : '存在你无法访问的题目，暂不能修改编排。请先向题目 owner 申请权限；已有条目不会被删除。'}
            </p>
            {set.permissions.editItems && (
              <Button onClick={handleSaveItems} loading={saving}>
                <Save />
                保存题目编排
              </Button>
            )}
          </div>
        )}
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-12">#</TableHead>
              {user ? <TableHead className="w-12" /> : null}
              <TableHead>题目</TableHead>
              <TableHead className="hidden w-64 lg:table-cell">备注</TableHead>
              <TableHead className="hidden w-16 text-right sm:table-cell">难度</TableHead>
              <TableHead className="hidden w-24 text-right sm:table-cell">通过率</TableHead>
              {editing && set.permissions.editItems ? (
                <TableHead className="w-32 text-right">调整</TableHead>
              ) : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.length === 0 ? (
              <TableEmpty colSpan={7}>
                <EmptyState
                  title="没有可见题目"
                  description={
                    set.permissions.editItems
                      ? '点击「编辑」后添加题目。'
                      : '题单为空，或其中题目尚未向你开放。'
                  }
                />
              </TableEmpty>
            ) : (
              items.map((entry, index) => (
                <TableRow key={entry.problemId}>
                  <TableCell className="tabular-nums text-muted-foreground">{index + 1}</TableCell>
                  {user ? (
                    <TableCell>
                      <ProblemStatusIcon status={entry.userStatus} />
                    </TableCell>
                  ) : null}
                  <TableCell className="min-w-48">
                    <Link
                      to={`/problems/${entry.problemPublicId || entry.problemId}`}
                      className="block break-words font-medium hover:text-primary"
                    >
                      {entry.title}
                    </Link>
                    <div className="flex flex-wrap gap-1 pt-0.5">
                      {entry.tags?.slice(0, 3).map((tag) => (
                        <Badge key={tag} variant="outline">
                          {tag}
                        </Badge>
                      ))}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground sm:hidden">
                      难度 {entry.difficulty} · 通过率{' '}
                      {formatRatio(entry.acceptCount, entry.submitCount)}
                    </p>
                    {editing && set.permissions.editItems ? (
                      <div className="mt-2 lg:hidden">
                        <label htmlFor={`set-note-${entry.problemId}`} className="sr-only">
                          {entry.title} 的备注
                        </label>
                        <Input
                          id={`set-note-${entry.problemId}`}
                          value={entry.note}
                          placeholder="给做题人的提示"
                          onChange={(event) => updateNote(index, event.target.value)}
                        />
                      </div>
                    ) : entry.note ? (
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground lg:hidden">
                        {entry.note}
                      </p>
                    ) : null}
                  </TableCell>
                  <TableCell className="hidden text-sm text-muted-foreground lg:table-cell">
                    {editing && set.permissions.editItems ? (
                      <Input
                        value={entry.note}
                        placeholder="给做题人的提示"
                        aria-label={`${entry.title} 的备注`}
                        onChange={(event) => updateNote(index, event.target.value)}
                      />
                    ) : (
                      entry.note || '—'
                    )}
                  </TableCell>
                  <TableCell className="hidden text-right tabular-nums sm:table-cell">
                    {entry.difficulty}
                  </TableCell>
                  <TableCell className="hidden text-right tabular-nums text-muted-foreground sm:table-cell">
                    {formatRatio(entry.acceptCount, entry.submitCount)}
                  </TableCell>
                  {editing && set.permissions.editItems ? (
                    <TableCell>
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          aria-label="上移"
                          disabled={index === 0}
                          onClick={() => move(index, -1)}
                        >
                          <ArrowUp />
                        </Button>
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          aria-label="下移"
                          disabled={index === items.length - 1}
                          onClick={() => move(index, 1)}
                        >
                          <ArrowDown />
                        </Button>
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          aria-label="移除"
                          className="hover:text-destructive"
                          onClick={() =>
                            setItems((current) =>
                              current.filter((_, position) => position !== index),
                            )
                          }
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>

      <Dialog open={picker} onOpenChange={setPicker}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>添加题目</DialogTitle>
          </DialogHeader>
          <form
            className="flex gap-2"
            onSubmit={(event) => {
              event.preventDefault()
              void openPicker()
            }}
          >
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索题目标题"
            />
            <Button type="submit" variant="outline">
              搜索
            </Button>
          </form>
          <div className="flex max-h-[50vh] flex-col gap-1 overflow-y-auto">
            {pickerLoading ? (
              <div className="flex flex-col gap-2 py-1">
                {Array.from({ length: 4 }, (_, index) => (
                  <div key={index} className="h-10 animate-pulse rounded-sm bg-muted" />
                ))}
              </div>
            ) : pickerError ? (
              <div className="flex flex-col items-center gap-2 py-8 text-center text-sm">
                <p className="text-destructive" role="alert">
                  {pickerError}
                </p>
                <Button variant="outline" size="sm" onClick={openPicker}>
                  重新加载
                </Button>
              </div>
            ) : candidates.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">没有匹配的题目。</p>
            ) : (
              candidates.map((problem) => (
                <button
                  key={problem.id}
                  type="button"
                  className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-left hover:bg-accent"
                  onClick={() => addProblem(problem)}
                >
                  <span className="flex-1 truncate">{problem.title}</span>
                  <Badge variant="outline">难度 {problem.difficulty}</Badge>
                </button>
              ))
            )}
          </div>
          <DialogFooter>
            {!pickerLoading && !pickerError && (
              <Pagination
                page={pickerPage}
                size={20}
                total={pickerTotal}
                onChange={(page) => void loadCandidates(page)}
              />
            )}
            <Button variant="outline" onClick={() => setPicker(false)}>
              完成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleting} onOpenChange={setDeleting}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>删除题单「{set.title}」?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            只会删除这个清单,题目本身和你的提交记录都不受影响。
          </p>
          <DialogFooter>
            <Button variant="outline" disabled={deletePending} onClick={() => setDeleting(false)}>
              取消
            </Button>
            <Button variant="destructive" loading={deletePending} onClick={handleDelete}>
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
