import { useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import {
  ArrowLeft,
  BookOpen,
  Code2,
  FileText,
  ListChecks,
  Lock,
  MessageSquare,
  Pencil,
  RotateCcw,
  Send,
  ThumbsUp,
  Trash2,
} from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoContestProblemDetailResponse as ContestProblem,
  DtoEditorialSummaryResponse as Editorial,
  DtoProblemResponse as PracticeProblem,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { useDomain } from '@/domain/DomainContext'
import CodeEditor, { languageOptions, languageTemplates } from '@/components/CodeEditor'
import DiscussionSection from '@/components/DiscussionSection'
import JudgeResultPanel from '@/components/JudgeResultPanel'
import MdRenderer from '@/components/MdRenderer'
import ProblemStatusIcon from '@/components/ProblemStatusIcon'
import SplitPane from '@/components/SplitPane'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input, Textarea } from '@/components/ui/input'
import { EmptyState, Separator, Skeleton } from '@/components/ui/misc'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { useSubmission } from '@/hooks/useSubmission'
import { useCanonicalPath } from '@/hooks/useCanonicalPath'
import { matchesReference, problemHref } from '@/lib/routes'
import {
  apiError,
  difficultyLabel,
  formatDate,
  formatMemory,
  formatRatio,
  formatTime,
} from '@/lib/format'
import { cn } from '@/lib/utils'

/** Per-problem, per-language drafts survive navigation and reloads. */
function draftKey(problemId: string, language: string) {
  const prefix = import.meta.env.VITE_MOCK === 'true' ? 'vertex-mock-draft' : 'vertex-draft'
  return `${prefix}:${problemId}:${language}`
}

type ProblemView = {
  version: number
  id: string
  publicId: string
  contestId?: string
  contestPublicId?: string
  title: string
  statementMd: string
  difficulty: number
  source: string
  timeLimitMs: number
  memoryLimitKb: number
  judgeType: string
  tags: string[]
  visibility: string
  userStatus?: string
  submissionCount?: number
  acceptedCount?: number
  contestLabel?: string
  points?: number
  canEdit?: boolean
}

function problemView(value: PracticeProblem | ContestProblem): ProblemView {
  if ('problemId' in value) {
    return {
      version: value.version,
      id: value.problemId,
      publicId: value.problemPublicId,
      contestId: value.contestId,
      contestPublicId: value.contestPublicId,
      title: value.title,
      statementMd: value.statementMd,
      difficulty: value.difficulty,
      source: value.source,
      timeLimitMs: value.timeLimitMs,
      memoryLimitKb: value.memoryLimitKb,
      judgeType: value.judgeType,
      tags: value.tags,
      visibility: value.visibility,
      contestLabel: value.label,
      points: value.points,
    }
  }
  return { ...value, version: value.publishedVersion, canEdit: value.permissions.edit }
}

export default function ProblemDetailPage() {
  const { can } = useDomain()
  const {
    deleteApiDiscussionsPostId: deleteDiscussion,
    getApiContestsIdProblemsProblemId: getContestProblem,
    putApiDiscussionsPostId: updateDiscussion,
    getApiProblemsId: getProblem,
    getApiProblemsIdDiscussions: listProblemDiscussions,
    postApiProblemsIdDiscussions: createProblemDiscussion,
    postApiSubmissions: submit,
  } = useDomainAPI()
  const { id, contestId: routeContestId } = useParams<{ id: string; contestId: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const confirm = useConfirm()
  const { user, ready } = useAuth()
  const [searchParams] = useSearchParams()
  const contestId = routeContestId ?? searchParams.get('contest') ?? undefined

  const [problem, setProblem] = useState<ProblemView | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const [language, setLanguage] = useState('cpp')
  const [code, setCode] = useState('')
  const [draftState, setDraftState] = useState<'saving' | 'saved' | 'unavailable'>('saving')
  const [submitting, setSubmitting] = useState(false)
  const [submissionId, setSubmissionId] = useState<string | undefined>()
  const [mobilePane, setMobilePane] = useState<'read' | 'code' | 'result'>('read')
  const submitRef = useRef<() => void>(() => undefined)
  const submitInFlightRef = useRef(false)
  const submitControllerRef = useRef<AbortController | null>(null)

  const { submission, setSubmission } = useSubmission(submissionId)
  const problemPath = id ? problemHref({ problemId: id, contestId }) : '/problems'
  const loadedCurrent =
    problem &&
    (matchesReference(id, problem) || (!!contestId && id === problem.contestLabel)) &&
    (contestId
      ? contestId === problem.contestId || contestId === problem.contestPublicId
      : !problem.contestId)
  useCanonicalPath(
    loadedCurrent
      ? problemHref({
          problemId: problem.id,
          problemPublicId: problem.publicId,
          contestId: problem.contestId,
          contestPublicId: problem.contestPublicId,
          label: problem.contestLabel,
        })
      : undefined,
    !!contestId,
  )
  const problemContext = `${user?.id ?? 'anonymous'}:${contestId ?? 'practice'}:${id ?? ''}`
  const activeProblemContext = useRef(problemContext)

  useEffect(() => {
    activeProblemContext.current = problemContext
    submitControllerRef.current?.abort()
    submitControllerRef.current = null
    submitInFlightRef.current = false
    setSubmitting(false)
    setSubmissionId(undefined)
    setSubmission(null)
    setMobilePane('read')
  }, [problemContext, setSubmission])

  useEffect(() => {
    // See ProblemListPage: userStatus is only correct once the access token
    // has been restored, so hold the request until the session is ready.
    if (!id || !ready) return
    if (contestId && !user) {
      setProblem(null)
      setLoadError('登录后才能打开比赛题目。')
      setLoading(false)
      return
    }
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    setProblem(null)
    const request = contestId
      ? getContestProblem(contestId, id, { signal: controller.signal })
      : getProblem(id, { signal: controller.signal })
    request
      .then((result) => {
        if (!controller.signal.aborted) setProblem(problemView(result))
      })
      .catch((error) => {
        if (controller.signal.aborted) return
        const message = apiError(error, '题目加载失败')
        setLoadError(message)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contestId, id, ready, user?.id, reloadToken])

  // Restore the draft for this problem/language, falling back to the template.
  useEffect(() => {
    if (!problem?.id) return
    try {
      const stored = localStorage.getItem(draftKey(problem.id, language))
      setCode(stored ?? languageTemplates[language] ?? '')
    } catch {
      setCode(languageTemplates[language] ?? '')
    }
  }, [problem?.id, language])

  useEffect(() => {
    if (!problem?.id) return
    setDraftState('saving')
    const timer = window.setTimeout(() => {
      try {
        localStorage.setItem(draftKey(problem.id, language), code)
        setDraftState('saved')
      } catch {
        setDraftState('unavailable')
      }
    }, 400)
    return () => window.clearTimeout(timer)
  }, [code, problem?.id, language])

  useEffect(() => {
    if (problem) document.title = `${problem.title} · Vertex`
  }, [problem])

  async function handleSubmit() {
    if (!problem) return
    if (!problem.version) {
      toast.warning('此题尚未发布可评测版本')
      return
    }
    if (!user) {
      toast.warning('请先登录后再提交')
      navigate('/login', { state: { from: problemPath } })
      return
    }
    if (!can('submission.create')) {
      toast.warning('当前域没有提交权限')
      return
    }
    if (!code.trim()) {
      toast.warning('请输入代码')
      return
    }
    if (submitInFlightRef.current) return

    const requestContext = problemContext
    const controller = new AbortController()
    submitInFlightRef.current = true
    submitControllerRef.current = controller
    setSubmitting(true)
    try {
      const created = await submit(
        {
          problemId: problem.id,
          language,
          sourceCode: code,
          contestId: problem.contestId,
        },
        { signal: controller.signal },
      )
      if (controller.signal.aborted || activeProblemContext.current !== requestContext) return
      setSubmission(created)
      setSubmissionId(created.id)
      setMobilePane('result')
    } catch (error) {
      if (controller.signal.aborted) return
      toast.error(apiError(error, '提交失败'))
    } finally {
      if (submitControllerRef.current === controller) {
        submitControllerRef.current = null
        submitInFlightRef.current = false
        setSubmitting(false)
      }
    }
  }

  submitRef.current = () => void handleSubmit()

  useEffect(() => {
    const submitFromEditor = (event: KeyboardEvent) => {
      const target = event.target
      if (!(target instanceof HTMLElement) || !target.closest('.cm-editor')) return
      if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
        event.preventDefault()
        void submitRef.current()
      }
    }
    window.addEventListener('keydown', submitFromEditor)
    return () => window.removeEventListener('keydown', submitFromEditor)
  }, [])

  async function restoreTemplate() {
    const template = languageTemplates[language] ?? ''
    if (
      code !== template &&
      !(await confirm({
        title: '恢复初始模板？',
        description: '当前草稿会被覆盖，且无法撤销。',
        confirmLabel: '恢复模板',
        destructive: true,
      }))
    )
      return
    setCode(template)
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
        title="无法打开题目"
        description={loadError || '它可能已被删除，或者你没有查看权限。'}
        action={
          <div className="flex gap-2">
            {contestId && !user ? (
              <Button asChild>
                <Link to="/login" state={{ from: problemPath }}>
                  登录后继续
                </Link>
              </Button>
            ) : null}
            {loadError && (!contestId || user) ? (
              <Button variant="outline" onClick={() => setReloadToken((value) => value + 1)}>
                重试
              </Button>
            ) : null}
            <Button variant="ghost" asChild>
              <Link to="/problems">返回题库</Link>
            </Button>
          </div>
        }
      />
    )
  }

  const difficulty = difficultyLabel(problem.difficulty)
  const backTo = contestId ? `/contests/${contestId}` : '/problems'
  const backLabel = contestId ? '返回比赛' : '返回题库'
  const communityAvailable = !contestId

  return (
    <div className="flex h-[calc(100dvh-var(--app-header-height))] min-h-0 flex-col lg:p-3">
      <div
        className="sticky top-[var(--app-header-height)] z-20 flex min-h-12 items-stretch border-b border-border bg-card lg:hidden"
        role="tablist"
        aria-label="工作区面板"
      >
        {(
          [
            ['read', FileText, '阅读'],
            ['code', Code2, '代码'],
            ['result', ListChecks, '结果'],
          ] as const
        ).map(([pane, Icon, label]) => (
          <button
            key={pane}
            type="button"
            role="tab"
            className={cn(
              'relative flex flex-1 items-center justify-center gap-1.5 text-sm text-muted-foreground',
              mobilePane === pane &&
                'text-foreground after:absolute after:inset-x-5 after:bottom-0 after:h-0.5 after:bg-primary',
            )}
            onClick={() => {
              setMobilePane(pane)
              window.scrollTo({ top: 0 })
            }}
            aria-selected={mobilePane === pane}
          >
            <Icon className="size-4" /> {label}
          </button>
        ))}
      </div>

      <SplitPane
        className="min-h-0 flex-1"
        leftClassName={cn(mobilePane === 'read' ? 'flex' : 'hidden', 'lg:flex')}
        rightClassName={cn(mobilePane === 'read' ? 'hidden' : 'flex', 'lg:flex')}
        left={
          <Tabs
            defaultValue="statement"
            className="flex min-h-0 flex-1 flex-col overflow-hidden bg-card lg:rounded-xl lg:border lg:border-border"
          >
            <div className="flex min-h-12 items-center gap-2 border-b border-border px-3">
              <Button variant="ghost" size="icon-sm" asChild aria-label={backLabel}>
                <Link to={backTo}>
                  <ArrowLeft />
                </Link>
              </Button>
              <TabsList>
                <TabsTrigger value="statement">
                  <FileText /> 题面
                </TabsTrigger>
                {communityAvailable ? (
                  <>
                    <TabsTrigger value="editorials">
                      <BookOpen /> 题解
                    </TabsTrigger>
                    <TabsTrigger value="discussions">
                      <MessageSquare /> 讨论
                    </TabsTrigger>
                  </>
                ) : null}
              </TabsList>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto">
              <TabsContent
                value="statement"
                className="mx-auto max-w-3xl px-5 py-7 sm:px-7 sm:py-8"
              >
                <div className="flex flex-col gap-4">
                  <div className="flex items-start gap-2.5">
                    {problem.userStatus ? (
                      <ProblemStatusIcon status={problem.userStatus} className="mt-1.5 size-5" />
                    ) : null}
                    <div className="min-w-0 flex-1">
                      <p className="mb-2 text-xs text-muted-foreground">
                        {problem.contestLabel
                          ? `题目 ${problem.contestLabel}`
                          : problem.source || '练习题目'}
                      </p>
                      <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">
                        {problem.title}
                      </h1>
                    </div>
                    {problem.canEdit && !contestId && (
                      <Button variant="ghost" size="sm" asChild>
                        <Link to={`/authoring/${problem.publicId || problem.id}`}>
                          <Pencil />
                          编辑题目
                        </Link>
                      </Button>
                    )}
                  </div>

                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs">
                    <span className={cn('rounded-md px-2 py-1 font-medium', difficulty.className)}>
                      {difficulty.label} · {problem.difficulty}
                    </span>
                    {problem.tags?.map((tag) => (
                      <Link
                        key={tag}
                        to={`/problems?tag=${encodeURIComponent(tag)}`}
                        className="text-muted-foreground hover:text-primary hover:underline"
                      >
                        {tag}
                      </Link>
                    ))}
                  </div>

                  <dl className="grid grid-cols-2 gap-x-4 gap-y-3 border-y border-border py-3 text-sm sm:grid-cols-4">
                    <Stat label="时间限制" value={formatTime(problem.timeLimitMs)} />
                    <Stat label="内存限制" value={formatMemory(problem.memoryLimitKb)} />
                    {contestId ? (
                      <>
                        <Stat label="比赛题号" value={problem.contestLabel || '—'} />
                        <Stat label="分值" value={String(problem.points ?? 0)} />
                      </>
                    ) : (
                      <>
                        <Stat label="提交" value={String(problem.submissionCount ?? 0)} />
                        <Stat
                          label="通过率"
                          value={formatRatio(
                            problem.acceptedCount ?? 0,
                            problem.submissionCount ?? 0,
                          )}
                        />
                      </>
                    )}
                  </dl>

                  <MdRenderer content={problem.statementMd} />

                  <Separator className="my-2" />
                  <Link
                    to={`/submissions?problem=${problem.publicId || problem.id}${contestId ? `&contest=${contestId}` : ''}`}
                    className="inline-flex w-fit items-center gap-1.5 text-sm text-primary hover:underline"
                  >
                    <ListChecks className="size-4" />
                    查看该题全部提交
                  </Link>
                </div>
              </TabsContent>

              {communityAvailable ? (
                <>
                  <TabsContent
                    value="editorials"
                    forceMount
                    className="mx-auto max-w-3xl px-5 py-7 data-[state=inactive]:hidden"
                  >
                    <EditorialSection problemId={problem.id} />
                  </TabsContent>

                  <TabsContent
                    value="discussions"
                    forceMount
                    className="mx-auto max-w-3xl px-5 py-7 data-[state=inactive]:hidden"
                  >
                    <DiscussionSection
                      reloadKey={problem.id}
                      fetchPosts={() => listProblemDiscussions(problem.id)}
                      createPost={async (content, parentId) => {
                        await createProblemDiscussion(problem.id, {
                          contentMd: content,
                          parentId,
                        })
                      }}
                      onUpdate={async (postId, content) => {
                        await updateDiscussion(postId, { contentMd: content })
                      }}
                      onDelete={async (postId) => {
                        await deleteDiscussion(postId)
                      }}
                    />
                  </TabsContent>
                </>
              ) : null}
            </div>
          </Tabs>
        }
        right={
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-card lg:rounded-xl lg:border lg:border-border">
            <div
              className={cn(
                'min-h-0 flex-1 flex-col',
                mobilePane === 'result' ? 'hidden lg:flex' : 'flex',
              )}
            >
              <div className="flex min-h-12 items-center gap-2 border-b border-border bg-background px-3">
                <Select value={language} onValueChange={setLanguage}>
                  <SelectTrigger className="h-8 w-32" aria-label="编程语言">
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
                  onClick={() => void restoreTemplate()}
                  title="恢复初始模板"
                >
                  <RotateCcw /> 模板
                </Button>
                <span className="hidden text-xs text-muted-foreground sm:inline" aria-live="polite">
                  {draftState === 'saved'
                    ? '草稿已保存'
                    : draftState === 'unavailable'
                      ? '本地保存不可用，请复制代码'
                      : '正在保存…'}
                </span>
                <Button
                  size="sm"
                  className="ml-auto"
                  loading={submitting}
                  disabled={!problem.version || (!!user && !can('submission.create'))}
                  title={
                    !problem.version
                      ? '尚未发布可评测版本'
                      : user && !can('submission.create')
                        ? '当前域没有提交权限'
                        : `使用发布版本 v${problem.version}`
                  }
                  onClick={handleSubmit}
                >
                  <Send /> {import.meta.env.VITE_MOCK === 'true' ? '模拟提交' : '提交代码'}
                  <span className="hidden font-normal opacity-70 xl:inline">Ctrl ↵</span>
                </Button>
              </div>

              <div className="min-h-0 flex-1 p-2">
                <CodeEditor
                  value={code}
                  onChange={setCode}
                  language={language}
                  ariaLabel={`${problem.title} 源代码编辑器`}
                />
              </div>
            </div>

            <div
              className={cn(mobilePane === 'result' ? 'block' : 'hidden', 'lg:block')}
              aria-live="polite"
            >
              {submission ? (
                <JudgeResultPanel submission={submission} />
              ) : (
                <div className="border-t border-border px-5 py-8 text-center text-sm text-muted-foreground">
                  提交后，这里会显示判题进度和测试点结果。
                </div>
              )}
            </div>
          </div>
        }
      />
    </div>
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
  const { can } = useDomain()
  const {
    getApiEditorials: listEditorials,
    postApiEditorials: createEditorial,
    postApiEditorialsIdVote: voteEditorial,
    deleteApiEditorialsId: removeEditorial,
  } = useDomainAPI()
  const { user } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()
  const [editorials, setEditorials] = useState<Editorial[]>([])
  // Anti-spoiler switch: hide the body until the reader solved the problem.
  const [solvedOnly, setSolvedOnly] = useState(false)
  const [loading, setLoading] = useState(true)
  const [composing, setComposing] = useState(false)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const requestSequence = useRef(0)

  async function load() {
    const sequence = ++requestSequence.current
    try {
      const result = await listEditorials({ problem: problemId })
      if (sequence !== requestSequence.current) return
      setEditorials(result.items)
    } catch (error) {
      if (sequence !== requestSequence.current) return
      toast.error(apiError(error, '题解加载失败'))
    } finally {
      if (sequence === requestSequence.current) setLoading(false)
    }
  }

  useEffect(() => {
    setLoading(true)
    void load()
    return () => {
      requestSequence.current += 1
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [problemId])

  async function handlePublish() {
    if (!title.trim() || !content.trim()) {
      toast.warning('请填写标题与内容')
      return
    }
    setSubmitting(true)
    try {
      await createEditorial({
        problemId,
        title: title.trim(),
        contentMd: content.trim(),
        solvedOnly,
      })
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

  async function handleVote(editorial: Editorial) {
    try {
      await voteEditorial(editorial.id, { up: !editorial.voted })
      await load()
    } catch (error) {
      toast.error(apiError(error, '操作失败'))
    }
  }

  async function handleDelete(editorial: Editorial) {
    const accepted = await confirm({
      title: '删除这篇题解？',
      description: '题解正文及其讨论会被永久删除。',
      confirmLabel: '删除题解',
      destructive: true,
    })
    if (!accepted) return
    try {
      await removeEditorial(editorial.id)
      toast.success('题解已删除')
      await load()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    }
  }

  if (loading) return <Skeleton className="h-40 w-full" />

  return (
    <div className="flex flex-col gap-5">
      {user && can('content.create') ? (
        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">题解 ({editorials.length})</p>
            <div className="flex items-center gap-1">
              <Button variant="ghost" size="sm" asChild>
                <Link to={`/editorials?problem=${problemId}`}>全部题解</Link>
              </Button>
              <Button variant="outline" size="sm" onClick={() => setComposing((open) => !open)}>
                {composing ? '取消' : '写题解'}
              </Button>
            </div>
          </div>
          {composing ? (
            <div className="flex flex-col gap-2 border-y border-border py-4">
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
              <div className="flex items-center justify-between">
                <label className="flex items-center gap-2 text-sm text-muted-foreground">
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked={solvedOnly}
                    onChange={(event) => setSolvedOnly(event.target.checked)}
                  />
                  通过本题后才能阅读（防剧透）
                </label>
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
        <div className="divide-y divide-border border-y border-border">
          {editorials.map((editorial) => (
            <article key={editorial.id} className="py-5">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <h3 className="text-base font-semibold">{editorial.title}</h3>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {editorial.authorName} · {formatDate(editorial.createdAt)}
                  </p>
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    size="sm"
                    variant={editorial.voted ? 'secondary' : 'outline'}
                    disabled={!editorial.permissions.vote}
                    onClick={() => handleVote(editorial)}
                  >
                    <ThumbsUp />
                    {editorial.voteCount}
                  </Button>
                  {editorial.permissions.delete ? (
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="删除题解"
                      className="hover:text-destructive"
                      onClick={() => handleDelete(editorial)}
                    >
                      <Trash2 />
                    </Button>
                  ) : null}
                </div>
              </div>
              {editorial.locked ? (
                <div className="mt-3 flex items-center gap-2 border-l-2 border-border py-1 pl-3 text-sm text-muted-foreground">
                  <Lock className="size-4" />
                  作者设置了「通过后可见」，先自己做出来再回来看。
                </div>
              ) : (
                <p className="mt-3 line-clamp-3 text-sm leading-6 text-muted-foreground">
                  打开独立阅读页查看完整思路、复杂度分析与讨论。
                </p>
              )}
              <Link
                to={`/editorials/${editorial.publicId || editorial.id}`}
                className="mt-3 inline-flex text-sm text-primary hover:underline"
              >
                阅读完整题解与讨论 →
              </Link>
            </article>
          ))}
        </div>
      )}
    </div>
  )
}
