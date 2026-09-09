import { useEffect, useLayoutEffect, useRef, useState } from 'react'
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
import ProblemSubmissionHistory from '@/components/ProblemSubmissionHistory'
import SplitPane from '@/components/SplitPane'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input, Textarea } from '@/components/ui/input'
import { EmptyState, Skeleton } from '@/components/ui/misc'
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
import { useContestSpace } from '@/components/contest/ContestContext'
import { showContestProblemMetadata } from '@/lib/contest-metadata'

/** Per-problem, per-language drafts survive navigation and reloads. */
function draftKey(problemId: string, language: string) {
  const prefix = import.meta.env.VITE_MOCK === 'true' ? 'vertex-mock-draft' : 'vertex-draft'
  return `${prefix}:${problemId}:${language}`
}

const languagePreferenceKey = draftKey('preferences', 'language')

function preferredLanguage() {
  try {
    const saved = localStorage.getItem(languagePreferenceKey)
    return languageOptions.some((option) => option.value === saved) ? saved! : 'cpp'
  } catch {
    return 'cpp'
  }
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
  lastSubmissionId?: string
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
      difficulty: value.difficulty ?? 0,
      source: value.source,
      timeLimitMs: value.timeLimitMs,
      memoryLimitKb: value.memoryLimitKb,
      judgeType: value.judgeType,
      tags: value.tags ?? [],
      visibility: value.visibility,
      contestLabel: value.label,
      points: value.points,
      userStatus: value.userStatus,
      lastSubmissionId: value.lastSubmissionId,
    }
  }
  return { ...value, version: value.publishedVersion, canEdit: value.permissions.edit }
}

export default function ProblemDetailPage() {
  const contestSpace = useContestSpace()
  const { can, slug } = useDomain()
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
  const [language, setLanguage] = useState(preferredLanguage)
  const [code, setCode] = useState('')
  const [loadedDraft, setLoadedDraft] = useState('')
  const [draftState, setDraftState] = useState<'saving' | 'saved' | 'unavailable'>('saving')
  const [submitting, setSubmitting] = useState(false)
  const [submissionId, setSubmissionId] = useState<string | undefined>()
  const [mobilePane, setMobilePane] = useState<'read' | 'code' | 'result'>('read')
  const submitRef = useRef<() => void>(() => undefined)
  const submitInFlightRef = useRef(false)
  const submitControllerRef = useRef<AbortController | null>(null)

  const {
    submission,
    setSubmission,
    loading: resultLoading,
    error: resultError,
  } = useSubmission(submissionId)
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
  const loadingScope = useRef(`${user?.id}:${contestId ?? 'practice'}`)
  const lastSubmissionKey =
    problem && user
      ? draftKey(
          problem.id,
          `last-submission:${slug}:${user.id}:${problem.contestId ?? 'practice'}`,
        )
      : undefined
  useEffect(() => {
    if (!lastSubmissionKey || !loadedCurrent || loading) return
    try {
      setSubmissionId(
        problem?.lastSubmissionId || localStorage.getItem(lastSubmissionKey) || undefined,
      )
    } catch {
      setSubmissionId(problem?.lastSubmissionId)
    }
  }, [lastSubmissionKey, loadedCurrent, loading, problem?.lastSubmissionId])
  const refreshContest = contestSpace?.refresh
  useEffect(() => {
    if (submission?.id && refreshContest) refreshContest()
  }, [submission?.id, submission?.status, refreshContest])

  useEffect(() => {
    activeProblemContext.current = problemContext
    submitControllerRef.current?.abort()
    submitControllerRef.current = null
    submitInFlightRef.current = false
    setSubmitting(false)
    setSubmissionId(undefined)
    setSubmission(null)
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
    const scope = `${user?.id}:${contestId ?? 'practice'}`
    if (loadingScope.current !== scope) setProblem(null)
    loadingScope.current = scope
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
  useLayoutEffect(() => {
    if (!problem?.id) return
    setLoadedDraft(draftKey(problem.id, language))
    try {
      const stored = localStorage.getItem(draftKey(problem.id, language))
      setCode(stored ?? languageTemplates[language] ?? '')
      setDraftState('saved')
    } catch {
      setCode(languageTemplates[language] ?? '')
      setDraftState('unavailable')
    }
  }, [problem?.id, language])

  // Persist edits immediately so navigation and language switches cannot cancel
  // the last pending save. Only user edits/reset write; draft loading never does.
  function updateCode(value: string) {
    if (!loadedCurrent || loading || loadError) return
    setCode(value)
    if (!problem?.id) return
    try {
      localStorage.setItem(draftKey(problem.id, language), value)
      setDraftState('saved')
    } catch {
      setDraftState('unavailable')
    }
  }

  function changeLanguage(value: string) {
    setLanguage(value)
    try {
      localStorage.setItem(languagePreferenceKey, value)
    } catch {
      setDraftState('unavailable')
    }
  }

  useEffect(() => {
    if (problem) document.title = `${problem.title} · Vertex`
  }, [problem])

  async function handleSubmit() {
    if (!loadedCurrent || loading || loadError) return
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
    if (contestSpace && !contestSpace.details?.contest.permissions.submit) {
      toast.warning('当前比赛身份没有提交权限')
      return
    }
    const event = contestSpace?.details?.contest
    if (event && (Date.now() < Date.parse(event.beginAt) || Date.now() > Date.parse(event.endAt))) {
      toast.warning('比赛不在进行中')
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
      if (lastSubmissionKey) {
        try {
          localStorage.setItem(lastSubmissionKey, created.id)
        } catch {
          /* Current result remains visible. */
        }
      }
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
    updateCode(template)
  }

  if (loading && !problem) {
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
              <Link to={contestId ? `/contests/${contestId}/problems` : '/problems'}>
                {contestId ? '返回比赛' : '返回题库'}
              </Link>
            </Button>
          </div>
        }
      />
    )
  }

  const difficulty = difficultyLabel(problem.difficulty)
  const backTo = contestId ? `/contests/${contestId}/problems` : '/problems'
  const backLabel = contestId ? '返回题目列表' : '返回题库'
  const communityAvailable = !contestId
  const currentSubmission =
    submission?.problemId === problem.id &&
    submission.userId === user?.id &&
    (submission.contestId ?? '') === (problem.contestId ?? '')
      ? submission
      : null
  const displayedStatus =
    contestSpace?.details?.problems.find((item) => item.problemId === problem.id)?.userStatus ??
    problem.userStatus
  const progressStatus =
    displayedStatus === 'solved'
      ? 'solved'
      : currentSubmission?.status === 'Accepted'
        ? 'solved'
        : currentSubmission
          ? ['Pending', 'Judging', 'Submitted'].includes(currentSubmission.status)
            ? 'submitted'
            : 'attempted'
          : displayedStatus
  const contestEvent = contestSpace?.details?.contest
  const ownStatus =
    contestEvent &&
    !contestEvent.permissions.viewJury &&
    contestEvent.feedback === 'none' &&
    Date.now() <= Date.parse(contestEvent.endAt) &&
    progressStatus &&
    progressStatus !== 'none'
      ? 'submitted'
      : progressStatus
  const showProblemMetadata = !contestId || showContestProblemMetadata(contestEvent)
  const contestOpen =
    !contestEvent ||
    (Date.now() >= Date.parse(contestEvent.beginAt) && Date.now() <= Date.parse(contestEvent.endAt))

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
                <TabsTrigger value="history">
                  <ListChecks />
                  历史提交
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
              {contestSpace?.details && (
                <div
                  className="ml-auto flex min-w-0 gap-1 overflow-x-auto"
                  aria-label="切换比赛题目"
                >
                  {contestSpace.details.problems.map((item) => (
                    <Link
                      key={item.problemId}
                      to={problemHref({
                        ...item,
                        contestId,
                        contestPublicId: contestSpace.details?.contest.publicId,
                      })}
                      aria-current={
                        id === item.label || id === item.problemId || id === item.problemPublicId
                          ? 'page'
                          : undefined
                      }
                      className={cn(
                        'shrink-0 rounded px-2 py-1 text-xs font-medium',
                        id === item.label || id === item.problemId || id === item.problemPublicId
                          ? 'bg-primary/10 text-primary'
                          : 'text-muted-foreground hover:bg-muted',
                      )}
                    >
                      {item.label}
                    </Link>
                  ))}
                </div>
              )}
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto">
              <TabsContent
                value="statement"
                className="mx-auto max-w-3xl px-5 py-7 sm:px-7 sm:py-8"
              >
                {!loadedCurrent || loadError ? (
                  loadError ? (
                    <EmptyState
                      title="这道题暂时无法加载"
                      description={loadError}
                      action={
                        <Button
                          variant="outline"
                          onClick={() => setReloadToken((value) => value + 1)}
                        >
                          重试
                        </Button>
                      }
                    />
                  ) : (
                    <div role="status" aria-label="正在切换题目" className="flex flex-col gap-4">
                      <p className="text-sm text-muted-foreground">正在加载题目 {id}…</p>
                      <Skeleton className="h-7 w-48" />
                      <Skeleton className="h-20 w-full" />
                      <Skeleton className="h-40 w-full" />
                    </div>
                  )
                ) : (
                  <div className="flex flex-col gap-4">
                    <div className="flex items-start gap-2.5">
                      {ownStatus ? (
                        <ProblemStatusIcon status={ownStatus} className="mt-1.5 size-5" />
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

                    {showProblemMetadata && (
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs">
                        <span
                          className={cn('rounded-md px-2 py-1 font-medium', difficulty.className)}
                        >
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
                    )}

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
                  </div>
                )}
              </TabsContent>

              <TabsContent value="history" className="px-3 py-4 sm:px-5">
                {loadedCurrent && !loading ? (
                  <ProblemSubmissionHistory
                    key={`${contestId ?? 'practice'}:${problem.id}`}
                    problemId={problem.id}
                    contestId={problem.contestId}
                    readAll={
                      !contestId ||
                      !!contestSpace?.details?.contest.permissions.viewJury ||
                      (contestEvent?.submissionVisibility === 'during' &&
                        Date.now() >= Date.parse(contestEvent.beginAt)) ||
                      (contestEvent?.submissionVisibility === 'after_end' &&
                        Date.now() > Date.parse(contestEvent.endAt))
                    }
                  />
                ) : (
                  <p role="status" className="text-sm text-muted-foreground">
                    正在切换题目…
                  </p>
                )}
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
          <div className="@container flex min-h-0 flex-1 flex-col overflow-hidden bg-card lg:rounded-xl lg:border lg:border-border">
            <div
              className={cn(
                'min-h-0 flex-1 flex-col',
                mobilePane === 'result' ? 'hidden lg:flex' : 'flex',
              )}
            >
              <div className="flex min-h-12 shrink-0 flex-wrap items-center gap-2 border-b border-border bg-background px-3 py-2">
                <Select
                  value={language}
                  onValueChange={changeLanguage}
                  disabled={!loadedCurrent || loading || !!loadError}
                >
                  <SelectTrigger className="h-8 w-28 shrink-0 @[420px]:w-32" aria-label="编程语言">
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
                  aria-label="恢复初始模板"
                  disabled={
                    !loadedCurrent ||
                    loading ||
                    !!loadError ||
                    (!!contestSpace && !contestSpace.details?.contest.permissions.submit)
                  }
                >
                  <RotateCcw /> <span className="hidden @[420px]:inline">模板</span>
                </Button>
                <span
                  className={cn(
                    'text-xs text-muted-foreground',
                    draftState === 'unavailable'
                      ? 'order-last w-full'
                      : 'sr-only @[520px]:not-sr-only',
                  )}
                  aria-live="polite"
                >
                  {draftState === 'saved'
                    ? '草稿已保存'
                    : draftState === 'unavailable'
                      ? '本地保存不可用，请复制代码'
                      : '正在保存…'}
                </span>
                <Button
                  size="sm"
                  className="ml-auto shrink-0"
                  loading={submitting}
                  disabled={
                    !loadedCurrent ||
                    loading ||
                    !!loadError ||
                    !problem.version ||
                    !contestOpen ||
                    (!!user && !can('submission.create')) ||
                    (!!contestSpace && !contestSpace.details?.contest.permissions.submit)
                  }
                  title={
                    !problem.version
                      ? '尚未发布可评测版本'
                      : user && !can('submission.create')
                        ? '当前域没有提交权限'
                        : `使用发布版本 v${problem.version}`
                  }
                  onClick={handleSubmit}
                >
                  <Send />{' '}
                  {contestSpace && !contestSpace.details?.contest.permissions.submit
                    ? '只读查看'
                    : !contestOpen
                      ? '比赛未开放提交'
                      : contestSpace?.details?.contest.permissions.rejudge
                        ? '测试提交（不计排名）'
                        : import.meta.env.VITE_MOCK === 'true'
                          ? '模拟提交'
                          : '提交代码'}
                  <span className="hidden font-normal opacity-70 @[640px]:inline">Ctrl ↵</span>
                </Button>
              </div>

              <div className="min-h-0 flex-1">
                <CodeEditor
                  documentKey={`${contestId ?? 'practice'}:${loadedDraft}`}
                  className="rounded-none border-0"
                  value={code}
                  readOnly={
                    !loadedCurrent ||
                    loading ||
                    !!loadError ||
                    (!!contestSpace && !contestSpace.details?.contest.permissions.submit)
                  }
                  onChange={updateCode}
                  language={language}
                  ariaLabel={`${problem.title} 源代码编辑器`}
                />
              </div>
            </div>

            <div
              className={cn(
                'min-h-0 flex-1 overflow-y-auto overscroll-contain border-t border-border lg:block lg:h-32 lg:flex-none lg:shrink-0',
                mobilePane === 'result' ? 'block' : 'hidden',
              )}
              aria-label="提交结果"
              aria-live="polite"
            >
              {currentSubmission ? (
                <JudgeResultPanel
                  key={currentSubmission.id}
                  submission={currentSubmission}
                  feedback={
                    contestEvent &&
                    !contestEvent.permissions.viewJury &&
                    Date.now() <= Date.parse(contestEvent.endAt)
                      ? contestEvent.feedback
                      : 'full'
                  }
                />
              ) : (
                <div className="flex h-full items-center justify-center px-5 py-8 text-center text-sm text-muted-foreground">
                  {resultLoading && submissionId
                    ? '正在恢复最近一次提交…'
                    : resultError
                      ? '最近一次提交暂时无法读取，请稍后重试。'
                      : '提交后，这里会显示判题进度和测试点结果。'}
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
