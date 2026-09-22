import { useEffect, useRef, useState } from 'react'
import {
  RefreshCw,
  CheckCircle2,
  Circle,
  Play,
  ArrowRight,
  ChevronDown,
  CircleAlert,
  Eye,
} from 'lucide-react'
import type {
  DomainCheckRun,
  DomainCommitRelease,
  DomainContentCommit,
  DomainMaterialInspection,
  DomainMaterialView,
  DomainTreeEntry,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Link } from '@/domain/navigation'
import { Button } from '@/components/ui/button'
import { SaveButton } from '@/components/ui/save-button'
import { Choice } from './MaterialForm'
import { apiError, formatDateTime } from '@/lib/format'
import CopyReleasePanel from './CopyReleasePanel'
import { checkMaterialTarget } from './check-material-target'
import { checkMatchesRelease, selectionIsCurrentRelease } from './release-readiness'

const languageName = (value: string) =>
  value === 'zh' ? '中文' : value === 'en' ? 'English' : value || '未标注语言'

export default function ReleasesPanel({
  problemId,
  canPublish,
  canCheck,
  onBusy,
}: {
  problemId: string
  canPublish: boolean
  canCheck: boolean
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI()
  const actionLock = useRef(false)
  const [copyBusy, setCopyBusy] = useState(false)
  const [runningCheck, setRunningCheck] = useState<DomainCheckRun>()
  const [commits, setCommits] = useState<DomainContentCommit[]>([]),
    [checks, setChecks] = useState<DomainCheckRun[]>([]),
    [releases, setReleases] = useState<DomainCommitRelease[]>([])
  const [currentVersion, setCurrentVersion] = useState(0),
    [revision, setRevision] = useState(''),
    [checkId, setCheckId] = useState(''),
    [language, setLanguage] = useState('')
  const [materials, setMaterials] = useState<{
    key: string
    inspection: DomainMaterialInspection
    metadata: DomainMaterialView
    entries: DomainTreeEntry[]
  }>()
  const [loadedOverview, setLoadedOverview] = useState(''),
    [canEditMaterials, setCanEditMaterials] = useState(false),
    [publishing, setPublishing] = useState(false)
  const [error, setError] = useState(''),
    [selectionError, setSelectionError] = useState(''),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [refresh, setRefresh] = useState(0),
    [published, setPublished] = useState<DomainCommitRelease>()
  const overviewKey = `${problemId}:${refresh}`
  const selectionKey = `${overviewKey}:${revision}`
  const selection = materials?.key === selectionKey ? materials : undefined
  const inspection = selection?.inspection
  const metadata = selection?.metadata
  const entries = selection?.entries ?? []
  const statements = entries.filter((entry) => entry.kind === 'statement')
  const overviewReady = loadedOverview === overviewKey
  const selectionReady = inspection?.revision === Number(revision) && !!revision
  useEffect(() => {
    onBusy(busy || copyBusy)
    return () => onBusy(false)
  }, [busy, copyBusy, onBusy])
  useEffect(() => {
    let live = true
    setLoading(true)
    setError('')
    Promise.all([
      api.getApiAuthoringProblemsIdCommits(problemId, { limit: 100 }),
      api.getApiAuthoringProblemsIdChecks(problemId, { limit: 100 }),
      api.getApiAuthoringProblemsIdReleases(problemId),
      api.getApiAdminProblemsId(problemId),
    ])
      .then(([history, results, versions, problem]) => {
        if (!live) return
        setCommits(history.items)
        setChecks(results.items)
        setReleases(versions.items)
        setCurrentVersion(problem.publishedVersion)
        setCanEditMaterials(problem.permissions.edit)
        setRevision((value) =>
          history.items.some((item) => String(item.revision) === value)
            ? value
            : String(history.items[0]?.revision ?? ''),
        )
        setLoadedOverview(overviewKey)
        setError('')
      })
      .catch((error) => {
        if (live) setError(apiError(error, '发布信息加载失败'))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [api, problemId, overviewKey])
  useEffect(() => {
    if (!revision) return
    let live = true
    setMaterials(undefined)
    setSelectionError('')
    setPublished(undefined)
    Promise.all([
      api.getApiAuthoringProblemsIdInspection(problemId, { revision: Number(revision) }),
      api.getApiAuthoringProblemsIdMaterialsEntryId(problemId, 'problem', {
        revision: Number(revision),
      }),
      api.getApiAuthoringProblemsIdCommitsRevision(problemId, Number(revision)),
    ])
      .then(([report, material, detail]) => {
        if (!live) return
        if (report.revision !== Number(revision) || !material.metadata)
          throw new Error('提交校验结果或题目设置不完整，请重新读取提交。')
        setMaterials({
          key: selectionKey,
          inspection: report,
          metadata: material,
          entries: detail.tree.entries,
        })
        setLanguage((value) => value || material.metadata?.statementLanguage || '')
      })
      .catch((error) => {
        if (live) setSelectionError(apiError(error, '提交材料加载失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId, revision, selectionKey])
  const matches = checks.filter(
    (check) => check.state === 'succeeded' && checkMatchesRelease(check, inspection),
  )
  const effectiveCheck = checkId ? matches.find((item) => item.id === checkId) : matches[0]
  const pendingCheck = checks.find(
    (check) =>
      checkMatchesRelease(check, inspection) && ['queued', 'running'].includes(check.state),
  )
  useEffect(() => {
    if (pendingCheck && pendingCheck.id !== runningCheck?.id) setRunningCheck(pendingCheck)
  }, [pendingCheck, runningCheck?.id])
  const currentRelease = overviewReady
    ? releases.find((item) => item.version === currentVersion)
    : undefined
  const releasedSelection =
    overviewReady &&
    selectionReady &&
    selectionIsCurrentRelease(
      currentRelease,
      currentVersion,
      Number(revision),
      effectiveCheck?.id,
      language,
    )
      ? currentRelease
      : undefined
  const publishedSelection =
    published?.version === releasedSelection?.version ? published : undefined
  const requirements = metadata?.metadata?.requirements ?? []
  const statement = statements.find((item) => item.attributes.language === language)
  const ready = Boolean(
    overviewReady &&
    !loading &&
    selectionReady &&
    inspection?.canBuild &&
    inspection.revision === Number(revision) &&
    !inspection.publicationIssues.length &&
    !requirements.length &&
    effectiveCheck &&
    statement &&
    ['markdown', 'tex', 'pdf'].includes(statement.attributes.format),
  )
  useEffect(() => {
    if (!runningCheck || !['queued', 'running'].includes(runningCheck.state)) return
    let live = true,
      inflight = false
    const timer = window.setInterval(() => {
      if (inflight) return
      inflight = true
      api
        .getApiAuthoringProblemsIdChecksCheckId(problemId, runningCheck.id)
        .then((value) => {
          if (live) {
            setRunningCheck(value)
            setChecks((current) => [value, ...current.filter((c) => c.id !== value.id)])
          }
        })
        .catch((e) => {
          if (live) setError(apiError(e, '检查状态读取失败'))
        })
        .finally(() => {
          inflight = false
        })
    }, 1500)
    return () => {
      live = false
      window.clearInterval(timer)
    }
  }, [api, problemId, runningCheck?.id, runningCheck?.state])
  async function checkSelected() {
    if (
      actionLock.current ||
      busy ||
      copyBusy ||
      !canCheck ||
      !overviewReady ||
      !selectionReady ||
      !inspection?.canBuild ||
      checking
    )
      return
    actionLock.current = true
    setBusy(true)
    setError('')
    try {
      const result = await api.postApiAuthoringProblemsIdChecks(problemId, {
        revision: Number(revision),
      })
      setCheckId(result.id)
      setRunningCheck(result)
      setChecks((current) => [result, ...current.filter((c) => c.id !== result.id)])
    } catch (e) {
      setError(apiError(e, '无法开始检查'))
    } finally {
      actionLock.current = false
      setBusy(false)
    }
  }
  async function publish() {
    if (
      actionLock.current ||
      busy ||
      copyBusy ||
      !canPublish ||
      !effectiveCheck ||
      !ready ||
      releasedSelection
    )
      return
    actionLock.current = true
    setPublishing(true)
    setCheckId(effectiveCheck.id)
    setBusy(true)
    setError('')
    setPublished(undefined)
    try {
      const value = await api.postApiAuthoringProblemsIdReleases(problemId, {
        revision: Number(revision),
        checkId: effectiveCheck.id,
        expectedVersion: currentVersion,
        language,
      })
      setPublished(value)
      setCurrentVersion(value.version)
      setReleases((current) => [value, ...current.filter((item) => item.version !== value.version)])
    } catch (error) {
      setError(apiError(error, '发布失败，原公开版本未改变。若版本已变化，请刷新后核对。'))
    } finally {
      actionLock.current = false
      setPublishing(false)
      setBusy(false)
    }
  }
  const checking =
    !!runningCheck &&
    checkMatchesRelease(runningCheck, inspection) &&
    ['queued', 'running'].includes(runningCheck.state)
  const selectedCommit = commits.find((item) => item.revision === Number(revision))
  const reportCheck =
    effectiveCheck ??
    (runningCheck && checkMatchesRelease(runningCheck, inspection) ? runningCheck : undefined)
  const checkLink = `/authoring/${problemId}/checks?${new URLSearchParams({ ...(revision ? { revision } : {}), ...(reportCheck ? { check: reportCheck.id } : {}) })}`
  const supportedStatement =
    statement && ['markdown', 'tex', 'pdf'].includes(statement.attributes.format)
  const blockers = [
    ...(!overviewReady ? ['请先刷新并取得最新发布信息'] : []),
    ...(!selectionReady ? [selectionError || '正在读取并核对所选提交'] : []),
    ...(selectionReady && !inspection?.canBuild ? ['提交材料尚未满足运行检查的条件'] : []),
    ...(selectionReady && !effectiveCheck
      ? [
          checkId
            ? '所选检查已不匹配当前材料或检查规则，请重新选择'
            : checking
              ? '匹配的检查正在运行，完成后继续'
              : '还没有与当前数据及检查规则匹配的成功检查',
        ]
      : []),
    ...(selectionReady && !supportedStatement
      ? ['请选择一份可发布的 Markdown、TeX 或 PDF 题面']
      : []),
    ...(inspection?.publicationIssues.map((issue) => issue.message) ?? []),
    ...requirements.map((requirement) => requirement.message),
  ]
  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">发布版本</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            把检查通过的提交交付给题库。现有比赛继续使用原来指定的版本。
          </p>
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={busy || copyBusy || loading}
          onClick={() => {
            setLoading(true)
            setPublished(undefined)
            setSelectionError('')
            setRefresh((value) => value + 1)
          }}
        >
          <RefreshCw className={loading ? 'animate-spin' : undefined} />
          {loading ? '刷新中' : '刷新'}
        </Button>
      </header>
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/25 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {!commits.length ? (
        !error && (
          <div className="rounded-xl border border-dashed py-16 text-center">
            <p className="text-sm font-medium">{loading ? '正在读取版本…' : '先记录第一次提交'}</p>
            <p className="mt-2 text-xs text-muted-foreground">
              提交确定这次要交付的内容，之后再运行检查和发布。
            </p>
            {!loading && (
              <Button asChild variant="outline" className="mt-5">
                <Link to={`/authoring/${problemId}/changes`}>去审阅更改</Link>
              </Button>
            )}
          </div>
        )
      ) : (
        <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_260px]">
          <div className="overflow-hidden rounded-xl border bg-card">
            <section className="space-y-4 border-b p-5 sm:p-6">
              <div className="flex items-center gap-3">
                <span className="flex size-7 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
                  1
                </span>
                <h3 className="text-sm font-semibold">确定要发布的内容</h3>
              </div>
              <Choice
                label="选择提交"
                value={revision}
                onChange={(value) => {
                  setRevision(value)
                  setCheckId('')
                  setLanguage('')
                  setPublished(undefined)
                  setSelectionError('')
                }}
                options={commits.map((item) => [
                  String(item.revision),
                  `r${item.revision} · ${item.message}`,
                ])}
                disabled={busy || copyBusy || loading}
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="rounded-lg border bg-muted/20 p-4">
                  <p className="text-xs text-muted-foreground">题库当前使用</p>
                  <p className="mt-2 text-xl font-semibold">
                    {overviewReady
                      ? currentVersion
                        ? `v${currentVersion}`
                        : '尚未发布'
                      : loading
                        ? '正在核对…'
                        : '待刷新确认'}
                  </p>
                  {currentRelease && overviewReady && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      来自 r{currentRelease.revision} · {languageName(currentRelease.language)}
                    </p>
                  )}
                  {currentVersion > 0 && overviewReady && (
                    <Link
                      className="mt-3 inline-flex items-center gap-1 text-xs text-primary hover:underline"
                      to={`/problems/${problemId}`}
                    >
                      <Eye className="size-3.5" />
                      预览当前版本
                    </Link>
                  )}
                </div>
                <div className="rounded-lg border border-primary/20 bg-primary/5 p-4">
                  <p className="text-xs text-muted-foreground">本次选择</p>
                  <p className="mt-2 text-xl font-semibold">提交 r{revision}</p>
                  <p className="mt-1 break-words text-sm">{selectedCommit?.message}</p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {currentRelease
                      ? currentRelease.revision === Number(revision)
                        ? '与当前线上版本来自同一次提交'
                        : `将线上内容从 r${currentRelease.revision} 更新为 r${revision}`
                      : '发布后成为题库当前版本'}
                  </p>
                </div>
              </div>
              {statements.length > 0 && (
                <Choice
                  label="题面语言"
                  value={language}
                  onChange={(value) => {
                    setLanguage(value)
                    setPublished(undefined)
                  }}
                  options={[
                    ...(!statement && language
                      ? [
                          [language, `${languageName(language)}（此提交缺少对应题面）`] as [
                            string,
                            string,
                          ],
                        ]
                      : []),
                    ...statements.map((item): [string, string] => [
                      item.attributes.language,
                      `${languageName(item.attributes.language)} · ${item.attributes.format?.toUpperCase() || '未知格式'}`,
                    ]),
                  ]}
                  disabled={busy || copyBusy || loading}
                />
              )}
              {selectionReady && !supportedStatement && (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-muted/30 p-3 text-xs">
                  <p className="text-muted-foreground">
                    此提交缺少所选语言的有效题面。可改选语言，或完善草稿题面后重新提交。
                  </p>
                  <Link
                    className="text-primary hover:underline"
                    to={`/authoring/${problemId}/statement`}
                  >
                    查看当前题面 →
                  </Link>
                </div>
              )}
              <p className="text-xs text-muted-foreground">
                {inspection
                  ? `${inspection.testCount} 个测试点 · ${inspection.sampleCount} 个公开样例 · ${inspection.programCount} 个程序`
                  : selectionError || '正在核对提交…'}
              </p>
              {selectionError && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={loading || busy || copyBusy}
                  onClick={() => setRefresh((value) => value + 1)}
                >
                  重试读取提交
                </Button>
              )}
            </section>
            <section className="space-y-4 border-b p-5 sm:p-6">
              <div className="flex items-center gap-3">
                <span className="flex size-7 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
                  2
                </span>
                <h3 className="text-sm font-semibold">确认检查结果</h3>
              </div>
              {effectiveCheck ? (
                <div className="flex items-start gap-3 rounded-lg bg-primary/5 p-4">
                  <CheckCircle2 className="size-5 shrink-0 text-primary" />
                  <div>
                    <p className="text-sm font-medium">所选提交的数据与程序已有匹配的成功检查</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {new Date(
                        effectiveCheck.finishedAt ?? effectiveCheck.createdAt,
                      ).toLocaleString()}
                      {statement?.attributes.format === 'tex'
                        ? ' · 将使用这次检查生成的 PDF 题面'
                        : ''}
                    </p>
                    {effectiveCheck.revision && effectiveCheck.revision !== Number(revision) && (
                      <p className="mt-2 text-xs text-muted-foreground">
                        检查来自 r{effectiveCheck.revision}
                        ；数据和检查规则与本次提交一致，可以复用。
                      </p>
                    )}
                  </div>
                </div>
              ) : (
                <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-muted/35 p-4">
                  <div className="flex items-center gap-3">
                    <Circle className="size-4 text-muted-foreground" />
                    <p className="text-sm">
                      {!selectionReady
                        ? selectionError
                          ? '提交校验信息未就绪'
                          : '正在核对提交…'
                        : checking
                          ? '正在运行匹配的检查…'
                          : '此提交还没有可用的成功检查'}
                    </p>
                  </div>
                  {canCheck && (
                    <Button
                      size="sm"
                      variant="outline"
                      loading={checking || busy}
                      disabled={
                        busy ||
                        copyBusy ||
                        loading ||
                        checking ||
                        !overviewReady ||
                        !selectionReady ||
                        !inspection?.canBuild
                      }
                      onClick={() => void checkSelected()}
                    >
                      <Play />
                      检查这个提交
                    </Button>
                  )}
                </div>
              )}
              {runningCheck?.state === 'failed' &&
                checkMatchesRelease(runningCheck, inspection) && (
                  <p role="alert" className="text-sm text-destructive">
                    {runningCheck.errorMessage || '检查未通过，请在运行检查页查看详细结果。'}
                  </p>
                )}
              {matches.length > 0 && (
                <div>
                  <div className="mt-3">
                    <Choice
                      label="成功检查"
                      value={effectiveCheck?.id ?? ''}
                      onChange={(v) => {
                        setCheckId(v)
                        setPublished(undefined)
                      }}
                      disabled={busy || copyBusy || loading}
                      options={[
                        ...(!effectiveCheck
                          ? [['', '请选择匹配的成功检查'] as [string, string]]
                          : []),
                        ...matches.map((item): [string, string] => [
                          item.id,
                          `${item.revision ? `r${item.revision}` : '草稿'} · ${formatDateTime(item.finishedAt ?? item.createdAt)}`,
                        ]),
                      ]}
                    />
                  </div>
                </div>
              )}
              {!!(
                inspection?.issues.some((item) => item.severity === 'error') ||
                inspection?.publicationIssues.length ||
                requirements.length
              ) && (
                <div className="space-y-3 rounded-lg border border-verdict-tle/30 bg-verdict-tle-bg p-4">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <CircleAlert className="size-4 text-verdict-tle" />
                    发布前需要处理
                  </p>
                  <ul className="space-y-3">
                    {[
                      ...(inspection?.issues.filter((item) => item.severity === 'error') ?? []),
                      ...(inspection?.publicationIssues ?? []),
                    ].map((item, index) => {
                      const entry = entries.find((entry) => entry.id === item.entryId)
                      const target =
                        canEditMaterials && entry
                          ? checkMaterialTarget(problemId, entry)
                          : undefined
                      return (
                        <li
                          key={`issue-${index}`}
                          className="flex flex-wrap items-start gap-2 text-sm"
                        >
                          <p className="min-w-0 flex-1 basis-48">{item.message}</p>
                          {target && (
                            <Link
                              className="text-xs leading-6 text-primary hover:underline"
                              to={target}
                            >
                              查看当前材料 →
                            </Link>
                          )}
                        </li>
                      )
                    })}
                    {requirements.map((item, index) => (
                      <li
                        key={`requirement-${index}`}
                        className="flex flex-wrap items-start gap-2 text-sm"
                      >
                        <p className="min-w-0 flex-1 basis-48">{item.message}</p>
                        <Link
                          className="text-xs leading-6 text-primary hover:underline"
                          to={`/authoring/${problemId}/overview?entry=problem`}
                        >
                          查看导入兼容项 →
                        </Link>
                      </li>
                    ))}
                  </ul>
                  <p className="text-xs text-muted-foreground">
                    修改草稿后，请重新提交，再选择新的提交发布。
                  </p>
                </div>
              )}
              <Link
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                to={checkLink}
              >
                查看完整检查报告
                <ArrowRight className="size-3" />
              </Link>
            </section>
            <section className="space-y-4 p-5 sm:p-6">
              <div className="flex items-center gap-3">
                <span className="flex size-7 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
                  3
                </span>
                <h3 className="text-sm font-semibold">创建发布版本</h3>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-4">
                <p className="text-sm text-muted-foreground">
                  {releasedSelection
                    ? `r${revision} 已发布为 v${releasedSelection.version}`
                    : ready
                      ? `将 r${revision} 的${languageName(language)}题面及检查产物发布到题库。`
                      : '完成上面的步骤后即可发布。'}
                </p>
                {canPublish ? (
                  <SaveButton
                    disabled={busy || copyBusy || !ready || !!releasedSelection}
                    loading={publishing}
                    saved={!!releasedSelection}
                    loadingLabel="发布中"
                    savedLabel={`已发布 v${releasedSelection?.version ?? ''}`}
                    onClick={() => void publish()}
                  >
                    {currentVersion ? '发布所选提交' : '发布首个版本'}
                  </SaveButton>
                ) : (
                  <span className="text-xs text-muted-foreground">需要发布权限</span>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                点击发布后才会更新题库当前版本。已开始的比赛继续使用原先指定的版本。
              </p>
              {releasedSelection && (
                <div
                  role="status"
                  className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-primary/5 p-4"
                >
                  <p className="text-sm text-primary">
                    {publishedSelection
                      ? `v${releasedSelection.version} 发布成功。`
                      : `这组提交、检查与题面已是当前 v${releasedSelection.version}。`}
                  </p>
                  <Button asChild variant="outline" size="sm">
                    <Link to={`/problems/${problemId}`}>
                      <Eye />
                      预览当前版本
                    </Link>
                  </Button>
                </div>
              )}
            </section>
          </div>
          <aside className="space-y-5 xl:sticky xl:top-[calc(var(--app-header-height)+1rem)]">
            <section aria-label="本次发布摘要" className="space-y-4 rounded-xl border bg-card p-5">
              <h3 className="text-sm font-semibold">本次发布摘要</h3>
              <dl className="space-y-3 text-xs">
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">提交</dt>
                  <dd className="font-medium">r{revision}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">题面</dt>
                  <dd>
                    {selectionReady && statement
                      ? `${languageName(language)} · ${statement.attributes.format?.toUpperCase() || '未知格式'}`
                      : '待选择 / 核对'}
                  </dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">评测数据</dt>
                  <dd>{selectionReady ? `${inspection?.testCount ?? 0} 个测试点` : '核对中'}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">公开样例</dt>
                  <dd>{selectionReady ? `${inspection?.sampleCount ?? 0} 个` : '核对中'}</dd>
                </div>
              </dl>
              <ul className="space-y-2 border-t pt-4 text-xs">
                {[
                  ['提交材料可检查', selectionReady && inspection?.canBuild],
                  ['检查结果匹配且通过', !!effectiveCheck && overviewReady],
                  ['题面已选择且可发布', !!supportedStatement && selectionReady],
                  [
                    '导入与发布要求已处理',
                    selectionReady && !requirements.length && !inspection?.publicationIssues.length,
                  ],
                ].map(([label, ok]) => (
                  <li key={String(label)} className="flex items-center gap-2">
                    {ok ? (
                      <CheckCircle2 className="size-4 shrink-0 text-primary" />
                    ) : (
                      <Circle className="size-4 shrink-0 text-muted-foreground" />
                    )}
                    <span>{label}</span>
                  </li>
                ))}
              </ul>
              {ready ? (
                <p role="status" className="rounded-lg bg-primary/5 p-3 text-xs text-primary">
                  {releasedSelection
                    ? '已是题库当前版本'
                    : canPublish
                      ? '已满足发布条件，确认后点击发布。'
                      : '内容已就绪，请有发布权限的协作者发布。'}
                </p>
              ) : (
                <div className="rounded-lg bg-muted/30 p-3">
                  <p className="text-xs font-medium">暂不能发布</p>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {blockers[0] ?? '请核对提交内容与检查结果。'}
                    {blockers.length > 1 ? `，另有 ${blockers.length - 1} 项待处理。` : ''}
                  </p>
                </div>
              )}
            </section>
            <details className="group rounded-xl border bg-card p-4" aria-label="发布历史">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-2 text-sm font-medium [&::-webkit-details-marker]:hidden">
                <span>发布历史 · {releases.length}</span>
                <ChevronDown className="size-4 text-muted-foreground transition-transform group-open:rotate-180" />
              </summary>
              <div className="mt-4 max-h-80 space-y-2 overflow-auto">
                {releases.map((item) => (
                  <div
                    key={item.version}
                    className="flex justify-between gap-3 border-l-2 py-2 pl-4"
                  >
                    <div>
                      <span className="text-sm font-medium">
                        v{item.version}
                        {overviewReady && item.version === currentVersion && (
                          <span className="ml-2 text-xs text-primary">当前</span>
                        )}
                      </span>
                      <p className="mt-1 text-xs text-muted-foreground">
                        r{item.revision} · {languageName(item.language)}
                      </p>
                    </div>
                    <time className="text-xs text-muted-foreground">
                      {new Date(item.createdAt).toLocaleDateString()}
                    </time>
                  </div>
                ))}
                {!releases.length && <p className="text-xs text-muted-foreground">暂无发布记录</p>}
              </div>
            </details>
          </aside>
        </div>
      )}
      {!!releases.length && (
        <details className="border-t pt-4">
          <summary className="cursor-pointer text-sm text-muted-foreground">
            复制已发布版本到其他题目
          </summary>
          <div className="mt-4">
            <CopyReleasePanel
              problemId={problemId}
              releases={releases}
              disabled={busy || loading || !overviewReady}
              onBusy={setCopyBusy}
            />
          </div>
        </details>
      )}
    </div>
  )
}
