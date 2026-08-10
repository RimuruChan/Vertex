import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ArrowLeft, BookOpen, FileText, ListChecks, MessageSquare, RotateCcw, Send } from 'lucide-react'
import {
  deleteApiDiscussionsId as deleteDiscussion,
  getApiEditorials as listEditorials,
  getApiProblemsId as getProblem,
  getApiProblemsIdDiscussions as listProblemDiscussions,
  postApiEditorials as createEditorial,
  postApiProblemsIdDiscussions as createProblemDiscussion,
  postApiSubmissions as submit,
} from '@/generated/api/vertex'
import type {
  DtoEditorialResponse as Editorial,
  DtoProblemResponse as Problem,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import CodeEditor, { languageOptions, languageTemplates } from '@/components/CodeEditor'
import DiscussionSection from '@/components/DiscussionSection'
import JudgeResultPanel from '@/components/JudgeResultPanel'
import MdRenderer from '@/components/MdRenderer'
import ProblemStatusIcon from '@/components/ProblemStatusIcon'
import SplitPane from '@/components/SplitPane'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { EmptyState, Separator, Skeleton } from '@/components/ui/misc'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { useSubmission } from '@/hooks/useSubmission'
import { apiError, difficultyLabel, formatDate, formatMemory, formatRatio, formatTime } from '@/lib/format'
import { cn } from '@/lib/utils'

/** Per-problem, per-language drafts survive navigation and reloads. */
function draftKey(problemId: string, language: string) {
  return `vertex-draft:${problemId}:${language}`
}

export default function ProblemDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const { user, ready } = useAuth()
  const [searchParams] = useSearchParams()
  const contestId = searchParams.get('contest') ?? undefined

  const [problem, setProblem] = useState<Problem | null>(null)
  const [loading, setLoading] = useState(true)
  const [language, setLanguage] = useState('cpp')
  const [code, setCode] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submissionId, setSubmissionId] = useState<string | undefined>()

  const { submission, setSubmission } = useSubmission(submissionId)

  useEffect(() => {
    // See ProblemListPage: userStatus is only correct once the access token
    // has been restored, so hold the request until the session is ready.
    if (!id || !ready) return
    setLoading(true)
    getProblem(id)
      .then(setProblem)
      .catch((error) => toast.error(apiError(error, '题目不存在')))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, ready, user?.id])

  // Restore the draft for this problem/language, falling back to the template.
  useEffect(() => {
    if (!id) return
    const stored = localStorage.getItem(draftKey(id, language))
    setCode(stored ?? languageTemplates[language] ?? '')
  }, [id, language])

  useEffect(() => {
    if (!id || !code) return
    const timer = window.setTimeout(() => localStorage.setItem(draftKey(id, language), code), 400)
    return () => window.clearTimeout(timer)
  }, [code, id, language])

  async function handleSubmit() {
    if (!id) return
    if (!user) {
      toast.warning('请先登录后再提交')
      navigate('/login', { state: { from: `/problems/${id}` } })
      return
    }
    if (!code.trim()) {
      toast.warning('请输入代码')
      return
    }
    setSubmitting(true)
    try {
      const created = await submit({ problemId: id, language, sourceCode: code, contestId })
      setSubmission(created)
      setSubmissionId(created.id)
    } catch (error) {
      toast.error(apiError(error, '提交失败'))
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-96 w-full" />
      </div>
    )
  }

  if (!problem) {
    return (
      <EmptyState
        title="题目不存在"
        description="它可能已被删除,或者你没有查看权限。"
        action={
          <Button variant="outline" asChild>
            <Link to="/problems">返回题库</Link>
          </Button>
        }
      />
    )
  }

  const difficulty = difficultyLabel(problem.difficulty)

  return (
    <SplitPane
      className="h-[calc(100vh-3.5rem)] min-h-0"
      left={
        <Tabs defaultValue="statement" className="flex min-h-0 flex-1 flex-col">
          <div className="flex items-center gap-2 border-b border-border px-4 py-2">
            <Button variant="ghost" size="icon-sm" asChild aria-label="返回题库">
              <Link to="/problems">
                <ArrowLeft />
              </Link>
            </Button>
            <TabsList>
              <TabsTrigger value="statement">
                <FileText />
                题面
              </TabsTrigger>
              <TabsTrigger value="editorials">
                <BookOpen />
                题解
              </TabsTrigger>
              <TabsTrigger value="discussions">
                <MessageSquare />
                讨论
              </TabsTrigger>
            </TabsList>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto">
            <TabsContent value="statement" className="px-5 py-5">
              <div className="flex flex-col gap-3">
                <div className="flex items-start gap-2.5">
                  <ProblemStatusIcon status={problem.userStatus} className="mt-1.5 size-5" />
                  <h1 className="text-xl font-semibold tracking-tight">{problem.title}</h1>
                </div>

                <div className="flex flex-wrap items-center gap-1.5">
                  <span className={cn('rounded-md px-2 py-0.5 text-xs font-medium', difficulty.className)}>
                    {difficulty.label} · {problem.difficulty}
                  </span>
                  {problem.tags?.map((tag) => (
                    <Link key={tag} to={`/problems?tag=${encodeURIComponent(tag)}`}>
                      <Badge variant="outline" className="hover:border-primary hover:text-primary">
                        {tag}
                      </Badge>
                    </Link>
                  ))}
                </div>

                <dl className="grid grid-cols-2 gap-x-4 gap-y-2 rounded-lg border border-border bg-muted/40 px-4 py-3 text-sm sm:grid-cols-4">
                  <Stat label="时间限制" value={formatTime(problem.timeLimitMs)} />
                  <Stat label="内存限制" value={formatMemory(problem.memoryLimitKb)} />
                  <Stat label="提交" value={String(problem.submissionCount)} />
                  <Stat
                    label="通过率"
                    value={formatRatio(problem.acceptedCount, problem.submissionCount)}
                  />
                </dl>

                <Separator className="my-1" />
                <MdRenderer content={problem.statementMd} />

                <Separator className="my-2" />
                <Link
                  to={`/submissions?problem=${problem.id}`}
                  className="inline-flex w-fit items-center gap-1.5 text-sm text-primary hover:underline"
                >
                  <ListChecks className="size-4" />
                  查看该题全部提交
                </Link>
              </div>
            </TabsContent>

            <TabsContent value="editorials" className="px-5 py-5">
              <EditorialSection problemId={problem.id} />
            </TabsContent>

            <TabsContent value="discussions" className="px-5 py-5">
              <DiscussionSection
                fetchPosts={async () => (await listProblemDiscussions(problem.id)).items}
                createPost={async (content, parentId) => {
                  await createProblemDiscussion(problem.id, { contentMd: content, parentId })
                }}
                onDelete={async (postId) => {
                  await deleteDiscussion(postId)
                }}
              />
            </TabsContent>
          </div>
        </Tabs>
      }
      right={
        <div className="flex min-h-0 flex-1 flex-col bg-muted/30">
          <div className="flex items-center gap-2 border-b border-border px-4 py-2">
            <Select value={language} onValueChange={setLanguage}>
              <SelectTrigger className="h-8 w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {languageOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setCode(languageTemplates[language] ?? '')}
              title="恢复初始模板"
            >
              <RotateCcw />
              模板
            </Button>
            <Button size="sm" className="ml-auto" loading={submitting} onClick={handleSubmit}>
              <Send />
              提交
            </Button>
          </div>

          <div className="min-h-64 flex-1 p-3">
            <CodeEditor value={code} onChange={setCode} language={language} />
          </div>

          {submission ? <JudgeResultPanel submission={submission} /> : null}
        </div>
      }
    />
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-medium tabular-nums">{value}</dd>
    </div>
  )
}

function EditorialSection({ problemId }: { problemId: string }) {
  const { user } = useAuth()
  const toast = useToast()
  const [editorials, setEditorials] = useState<Editorial[]>([])
  const [loading, setLoading] = useState(true)
  const [composing, setComposing] = useState(false)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function load() {
    try {
      setEditorials((await listEditorials({ problem: problemId })).items)
    } catch (error) {
      toast.error(apiError(error, '题解加载失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [problemId])

  async function handlePublish() {
    if (!title.trim() || !content.trim()) {
      toast.warning('请填写标题与内容')
      return
    }
    setSubmitting(true)
    try {
      await createEditorial({ problemId, title: title.trim(), contentMd: content.trim() })
      toast.success('题解已发布')
      setComposing(false)
      setTitle('')
      setContent('')
      await load()
    } catch (error) {
      toast.error(apiError(error, '发布失败'))
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) return <Skeleton className="h-40 w-full" />

  return (
    <div className="flex flex-col gap-5">
      {user ? (
        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">题解 ({editorials.length})</p>
            <Button variant="outline" size="sm" onClick={() => setComposing((open) => !open)}>
              {composing ? '取消' : '写题解'}
            </Button>
          </div>
          {composing ? (
            <div className="flex flex-col gap-2 rounded-lg border border-border p-3">
              <Input
                placeholder="题解标题"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
              <Textarea
                rows={8}
                placeholder="题解内容,支持 Markdown 与 LaTeX"
                value={content}
                onChange={(event) => setContent(event.target.value)}
              />
              <div className="flex justify-end">
                <Button size="sm" loading={submitting} onClick={handlePublish}>
                  发布
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      ) : null}

      {editorials.length === 0 ? (
        <EmptyState
          icon={<BookOpen />}
          title="还没有题解"
          description="做出来了?把思路写下来,帮到下一个人。"
        />
      ) : (
        <div className="flex flex-col gap-4">
          {editorials.map((editorial) => (
            <article key={editorial.id} className="rounded-lg border border-border p-4">
              <h3 className="text-base font-semibold">{editorial.title}</h3>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {editorial.authorName} · {formatDate(editorial.createdAt)}
              </p>
              <MdRenderer content={editorial.contentMd} className="mt-3" />
            </article>
          ))}
        </div>
      )}
    </div>
  )
}
