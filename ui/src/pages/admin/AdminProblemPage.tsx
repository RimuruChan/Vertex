import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Eye, FileArchive, Plus, Search, Trash2 } from 'lucide-react'
import {
  deleteApiAdminProblemsId as adminDeleteProblem,
  getApiAdminProblems as adminListProblems,
  postApiAdminProblems as adminCreateProblem,
  postApiAdminProblemsIdTestdata as adminUploadTestdata,
  putApiAdminProblemsId as adminUpdateProblem,
} from '@/generated/api/vertex'
import type { DtoProblemResponse as Problem } from '@/generated/api/model'
import MdRenderer from '@/components/MdRenderer'
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { apiError, shortId } from '@/lib/format'

const PAGE_SIZE = 20
const ANY = 'any'

type ProblemDraft = {
  title: string
  source: string
  statementMd: string
  difficulty: number
  timeLimitMs: number
  memoryLimitKb: number
  visibility: string
  tags: string[]
}

const emptyDraft: ProblemDraft = {
  title: '',
  source: '',
  statementMd: '',
  difficulty: 1,
  timeLimitMs: 1000,
  memoryLimitKb: 262144,
  visibility: 'draft',
  tags: [],
}

const visibilities = [
  { value: 'draft', label: '草稿' },
  { value: 'private', label: '私有' },
  { value: 'public', label: '公开' },
]

function visibilityLabel(visibility: string): string {
  return visibilities.find((item) => item.value === visibility)?.label ?? visibility
}

export default function AdminProblemPage() {
  const toast = useToast()
  const [problems, setProblems] = useState<Problem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [search, setSearch] = useState('')
  const [visibility, setVisibility] = useState('')
  const [loading, setLoading] = useState(true)

  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<Problem | null>(null)
  const [draft, setDraft] = useState<ProblemDraft>(emptyDraft)
  const [tagInput, setTagInput] = useState('')
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState<Problem | null>(null)

  async function load() {
    setLoading(true)
    try {
      const result = await adminListProblems({
        page,
        size: PAGE_SIZE,
        keyword: keyword || undefined,
        visibility: visibility || undefined,
      })
      setProblems(result.items)
      setTotal(result.total)
    } catch (error) {
      toast.error(apiError(error, '题目列表加载失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, keyword, visibility])

  function openCreate() {
    setEditing(null)
    setDraft(emptyDraft)
    setTagInput('')
    setEditorOpen(true)
  }

  function openEdit(problem: Problem) {
    setEditing(problem)
    setDraft({
      title: problem.title,
      source: problem.source ?? '',
      statementMd: problem.statementMd ?? '',
      difficulty: problem.difficulty,
      timeLimitMs: problem.timeLimitMs,
      memoryLimitKb: problem.memoryLimitKb,
      visibility: problem.visibility,
      tags: problem.tags ?? [],
    })
    setTagInput('')
    setEditorOpen(true)
  }

  async function handleSave() {
    if (!draft.title.trim()) {
      toast.warning('请填写题目标题')
      return
    }
    if (!draft.statementMd.trim()) {
      toast.warning('请填写题面')
      return
    }
    setSaving(true)
    try {
      if (editing) {
        await adminUpdateProblem(editing.id, draft)
        toast.success('题目已更新')
      } else {
        await adminCreateProblem(draft)
        toast.success('题目已创建,记得上传测试数据')
      }
      setEditorOpen(false)
      await load()
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    if (!deleting) return
    try {
      await adminDeleteProblem(deleting.id)
      toast.success('已删除')
      setDeleting(null)
      await load()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    }
  }

  function addTag() {
    const tag = tagInput.trim()
    if (!tag || draft.tags.includes(tag)) {
      setTagInput('')
      return
    }
    setDraft((current) => ({ ...current, tags: [...current.tags, tag] }))
    setTagInput('')
  }

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">出题管理</h1>
          <p className="text-sm text-muted-foreground">共 {total} 道题目(含草稿与私有)</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <form
            className="relative"
            onSubmit={(event) => {
              event.preventDefault()
              setPage(1)
              setKeyword(search.trim())
            }}
          >
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索标题或来源"
              className="w-52 pl-8"
            />
          </form>
          <Select
            value={visibility || ANY}
            onValueChange={(value) => {
              setPage(1)
              setVisibility(value === ANY ? '' : value)
            }}
          >
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>全部可见性</SelectItem>
              {visibilities.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button onClick={openCreate}>
            <Plus />
            新建题目
          </Button>
        </div>
      </div>

      <Card className="overflow-hidden">
        {loading ? (
          <div className="flex flex-col gap-2 p-4">
            {Array.from({ length: 6 }, (_, index) => (
              <Skeleton key={index} className="h-10 w-full" />
            ))}
          </div>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-24">编号</TableHead>
                  <TableHead>标题</TableHead>
                  <TableHead className="hidden w-40 lg:table-cell">来源</TableHead>
                  <TableHead className="w-24">可见性</TableHead>
                  <TableHead className="w-16 text-right">难度</TableHead>
                  <TableHead className="w-20 text-right">提交</TableHead>
                  <TableHead className="w-56 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {problems.length === 0 ? (
                  <TableEmpty colSpan={7}>
                    <EmptyState
                      title="还没有题目"
                      description="创建第一道题,然后上传测试数据。"
                      action={<Button onClick={openCreate}>新建题目</Button>}
                    />
                  </TableEmpty>
                ) : (
                  problems.map((problem) => (
                    <TableRow key={problem.id}>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        #{shortId(problem.id)}
                      </TableCell>
                      <TableCell className="max-w-0 truncate font-medium">{problem.title}</TableCell>
                      <TableCell className="hidden truncate text-muted-foreground lg:table-cell">
                        {problem.source || '—'}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            problem.visibility === 'public'
                              ? 'success'
                              : problem.visibility === 'private'
                                ? 'warning'
                                : 'secondary'
                          }
                        >
                          {visibilityLabel(problem.visibility)}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{problem.difficulty}</TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {problem.submissionCount}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-1">
                          <Button variant="outline" size="sm" onClick={() => openEdit(problem)}>
                            编辑
                          </Button>
                          <TestdataUploader problemId={problem.id} />
                          {problem.visibility === 'public' ? (
                            <Button variant="ghost" size="icon-sm" asChild aria-label="查看">
                              <Link to={`/problems/${problem.id}`}>
                                <Eye />
                              </Link>
                            </Button>
                          ) : null}
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label="删除"
                            className="hover:text-destructive"
                            onClick={() => setDeleting(problem)}
                          >
                            <Trash2 />
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
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{editing ? '编辑题目' : '新建题目'}</DialogTitle>
          </DialogHeader>

          <Tabs defaultValue="edit" className="flex flex-col gap-4">
            <TabsList>
              <TabsTrigger value="edit">编辑</TabsTrigger>
              <TabsTrigger value="preview">题面预览</TabsTrigger>
            </TabsList>

            <TabsContent value="edit" className="flex flex-col gap-3">
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="标题" id="problem-title">
                  <Input
                    id="problem-title"
                    value={draft.title}
                    onChange={(event) => setDraft({ ...draft, title: event.target.value })}
                    placeholder="题目名称"
                  />
                </Field>
                <Field label="来源" id="problem-source">
                  <Input
                    id="problem-source"
                    value={draft.source}
                    onChange={(event) => setDraft({ ...draft, source: event.target.value })}
                    placeholder="例如:原创、Codeforces Round 123"
                  />
                </Field>
              </div>

              <Field label="题面(Markdown + LaTeX)" id="problem-statement">
                <Textarea
                  id="problem-statement"
                  rows={12}
                  className="font-mono text-xs"
                  value={draft.statementMd}
                  onChange={(event) => setDraft({ ...draft, statementMd: event.target.value })}
                  placeholder="题目描述、输入格式、输出格式与样例;支持 $\sum$ 等公式"
                />
              </Field>

              <div className="grid gap-3 sm:grid-cols-4">
                <Field label="难度 1–10" id="problem-difficulty">
                  <Input
                    id="problem-difficulty"
                    type="number"
                    min={1}
                    max={10}
                    value={draft.difficulty}
                    onChange={(event) =>
                      setDraft({ ...draft, difficulty: Number(event.target.value) || 1 })
                    }
                  />
                </Field>
                <Field label="时间限制 (ms)" id="problem-time">
                  <Input
                    id="problem-time"
                    type="number"
                    min={100}
                    max={60000}
                    step={100}
                    value={draft.timeLimitMs}
                    onChange={(event) =>
                      setDraft({ ...draft, timeLimitMs: Number(event.target.value) || 1000 })
                    }
                  />
                </Field>
                <Field label="内存限制 (KB)" id="problem-memory">
                  <Input
                    id="problem-memory"
                    type="number"
                    min={16384}
                    max={4194304}
                    step={65536}
                    value={draft.memoryLimitKb}
                    onChange={(event) =>
                      setDraft({ ...draft, memoryLimitKb: Number(event.target.value) || 262144 })
                    }
                  />
                </Field>
                <Field label="可见性" id="problem-visibility">
                  <Select
                    value={draft.visibility}
                    onValueChange={(value) => setDraft({ ...draft, visibility: value })}
                  >
                    <SelectTrigger id="problem-visibility">
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
              </div>

              <Field label="标签" id="problem-tags">
                <div className="flex flex-col gap-2">
                  <Input
                    id="problem-tags"
                    value={tagInput}
                    onChange={(event) => setTagInput(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault()
                        addTag()
                      }
                    }}
                    placeholder="输入标签后回车"
                  />
                  {draft.tags.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {draft.tags.map((tag) => (
                        <button
                          key={tag}
                          type="button"
                          onClick={() =>
                            setDraft({ ...draft, tags: draft.tags.filter((item) => item !== tag) })
                          }
                        >
                          <Badge variant="secondary" className="hover:text-destructive">
                            {tag} ×
                          </Badge>
                        </button>
                      ))}
                    </div>
                  ) : null}
                </div>
              </Field>
            </TabsContent>

            <TabsContent value="preview">
              {draft.statementMd ? (
                <div className="max-h-[50vh] overflow-y-auto rounded-lg border border-border p-4">
                  <MdRenderer content={draft.statementMd} />
                </div>
              ) : (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  填写题面后可以在这里预览。
                </p>
              )}
            </TabsContent>
          </Tabs>

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

      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>确认删除「{deleting?.title}」?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            题目、测试数据和相关提交记录都会被删除,此操作不可撤销。
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              取消
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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

/** Testdata upload keeps its own state so one problem's upload does not block the table. */
function TestdataUploader({ problemId }: { problemId: string }) {
  const toast = useToast()
  const inputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)

  async function handleFile(file: File) {
    setUploading(true)
    try {
      const result = await adminUploadTestdata(problemId, { file, checker: 'diff' })
      toast.success(`上传成功:${result.caseCount} 个测试点`)
    } catch (error) {
      toast.error(apiError(error, '上传失败'))
    } finally {
      setUploading(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept=".zip"
        className="hidden"
        onChange={(event) => {
          const file = event.target.files?.[0]
          if (file) void handleFile(file)
        }}
      />
      <Button
        variant="outline"
        size="sm"
        loading={uploading}
        onClick={() => inputRef.current?.click()}
      >
        <FileArchive />
        数据
      </Button>
    </>
  )
}
