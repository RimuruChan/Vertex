import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  deleteApiAdminProblemsId as deleteProblem,
  getApiAdminProblemsId as getProblem,
  putApiAdminProblemsId as updateProblem,
} from '@/generated/api/vertex'
import type { DtoProblemResponse } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function SettingsPanel({
  problemId,
  onSaved,
}: {
  problemId: string
  onSaved: () => void
}) {
  const navigate = useNavigate()
  const confirm = useConfirm()
  const toast = useToast()
  const [problem, setProblem] = useState<DtoProblemResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [retry, setRetry] = useState(0)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [tags, setTags] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    setError(null)
    setProblem(null)
    getProblem(problemId, { signal: controller.signal })
      .then((value) => {
        if (!controller.signal.aborted) {
          setProblem(value)
          setTags(value.tags.join(', '))
        }
      })
      .catch((caught) => {
        if (!controller.signal.aborted) setError(apiError(caught, '设置加载失败'))
      })
    return () => controller.abort()
  }, [problemId, retry])
  if (error)
    return (
      <EmptyState
        title="设置加载失败"
        description={error}
        action={<Button onClick={() => setRetry((value) => value + 1)}>重试</Button>}
      />
    )
  if (!problem) return <Skeleton className="h-72" />
  async function save() {
    if (!problem || saving) return
    setSaving(true)
    try {
      // Refresh fields owned by the statement editor before updating settings.
      const latest = await getProblem(problemId)
      const value = await updateProblem(problemId, {
        title: latest.title,
        statementMd: latest.statementMd,
        difficulty: problem.difficulty,
        source: problem.source,
        timeLimitMs: problem.timeLimitMs,
        memoryLimitKb: problem.memoryLimitKb,
        visibility: problem.visibility,
        tags: [
          ...new Set(
            tags
              .split(/[,，]/)
              .map((tag) => tag.trim())
              .filter(Boolean),
          ),
        ],
      })
      setProblem(value)
      toast.success('设置已保存')
      onSaved()
    } catch (caught) {
      toast.error(apiError(caught, '保存失败'))
    } finally {
      setSaving(false)
    }
  }
  async function remove() {
    if (!problem || deleting) return
    if (
      !(await confirm({
        title: `删除「${problem.title}」？`,
        description:
          '题目及其工作副本、测试数据将永久删除。已有提交或比赛引用的题目不能删除，请改为隐藏。',
        confirmLabel: '删除题目',
        destructive: true,
      }))
    )
      return
    setDeleting(true)
    try {
      await deleteProblem(problemId)
      navigate('/authoring')
      toast.success('题目已删除')
    } catch (caught) {
      toast.error(apiError(caught, '删除失败'))
    } finally {
      setDeleting(false)
    }
  }
  return (
    <div className="space-y-5">
      <form
        className="surface-panel space-y-5 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <div>
          <h2 className="font-medium">题目设置</h2>
          <p className="mt-1 text-xs text-muted-foreground">题目标题和正文在「题面」中编辑。</p>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="setting-time">时间限制（ms）</Label>
            <Input
              id="setting-time"
              type="number"
              min={100}
              max={60000}
              required
              value={problem.timeLimitMs}
              onChange={(e) => setProblem({ ...problem, timeLimitMs: Number(e.target.value) })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="setting-memory">内存限制（KB）</Label>
            <Input
              id="setting-memory"
              type="number"
              min={16384}
              max={4194304}
              required
              value={problem.memoryLimitKb}
              onChange={(e) => setProblem({ ...problem, memoryLimitKb: Number(e.target.value) })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="setting-difficulty">难度</Label>
            <Input
              id="setting-difficulty"
              type="number"
              min={1}
              max={10}
              required
              value={problem.difficulty}
              onChange={(e) => setProblem({ ...problem, difficulty: Number(e.target.value) })}
            />
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="setting-source">来源</Label>
            <Input
              id="setting-source"
              value={problem.source}
              onChange={(e) => setProblem({ ...problem, source: e.target.value })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="setting-tags">标签（逗号分隔）</Label>
            <Input id="setting-tags" value={tags} onChange={(e) => setTags(e.target.value)} />
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="setting-visibility">可见性</Label>
          <Select
            value={problem.visibility}
            onValueChange={(visibility) => setProblem({ ...problem, visibility })}
          >
            <SelectTrigger id="setting-visibility" className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="draft">草稿</SelectItem>
              <SelectItem value="private">私有</SelectItem>
              <SelectItem value="public">公开</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Button type="submit" loading={saving}>
          保存设置
        </Button>
      </form>
      <section className="surface-panel flex flex-wrap items-center justify-between gap-4 border-destructive/30 p-5">
        <div>
          <h2 className="text-sm font-medium">删除题目</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            有提交或比赛引用时请改为隐藏，以保留历史版本；无引用题目删除后无法恢复。
          </p>
        </div>
        <Button variant="destructive" loading={deleting} onClick={() => void remove()}>
          删除题目
        </Button>
      </section>
    </div>
  )
}
