import { useEffect, useState } from 'react'
import { RefreshCw, CheckCircle2, Circle, Play, ArrowRight } from 'lucide-react'
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
import { apiError } from '@/lib/format'
import CopyReleasePanel from './CopyReleasePanel'

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
  const [copyBusy, setCopyBusy] = useState(false)
  const [runningCheck, setRunningCheck] = useState<DomainCheckRun>()
  const [commits, setCommits] = useState<DomainContentCommit[]>([]),
    [checks, setChecks] = useState<DomainCheckRun[]>([]),
    [releases, setReleases] = useState<DomainCommitRelease[]>([])
  const [currentVersion, setCurrentVersion] = useState(0),
    [revision, setRevision] = useState(''),
    [checkId, setCheckId] = useState(''),
    [language, setLanguage] = useState('')
  const [inspection, setInspection] = useState<DomainMaterialInspection>(),
    [metadata, setMetadata] = useState<DomainMaterialView>(),
    [statements, setStatements] = useState<DomainTreeEntry[]>([])
  const [error, setError] = useState(''),
    [selectionError, setSelectionError] = useState(''),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [refresh, setRefresh] = useState(0),
    [published, setPublished] = useState<DomainCommitRelease>()
  useEffect(() => {
    onBusy(busy || copyBusy)
    return () => onBusy(false)
  }, [busy, copyBusy, onBusy])
  useEffect(() => {
    let live = true
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
        setRevision((value) => value || String(history.items[0]?.revision ?? ''))
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
  }, [api, problemId, refresh])
  useEffect(() => {
    if (!revision) return
    let live = true
    setInspection(undefined)
    setMetadata(undefined)
    setStatements([])
    setCheckId('')
    setLanguage('')
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
        setInspection(report)
        setMetadata(material)
        setStatements(detail.tree.entries.filter((entry) => entry.kind === 'statement'))
        setLanguage(material.metadata?.statementLanguage ?? '')
      })
      .catch((error) => {
        if (live) setSelectionError(apiError(error, '提交材料加载失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId, revision])
  const matches = checks.filter(
    (check) =>
      check.state === 'succeeded' &&
      check.dataHash === inspection?.dataHash &&
      check.policyVersion === inspection?.policyVersion,
  )
  const effectiveCheck = matches.find((item) => item.id === checkId) ?? matches[0]
  const pendingCheck = checks.find(
    (check) =>
      check.dataHash === inspection?.dataHash &&
      check.policyVersion === inspection?.policyVersion &&
      ['queued', 'running'].includes(check.state),
  )
  useEffect(() => {
    if (pendingCheck && pendingCheck.id !== runningCheck?.id) setRunningCheck(pendingCheck)
  }, [pendingCheck, runningCheck?.id])
  const currentSelection = releases.find(
    (item) =>
      item.version === currentVersion &&
      item.revision === Number(revision) &&
      item.checkId === effectiveCheck?.id &&
      item.language === language,
  )
  const releasedSelection = published ?? currentSelection
  const requirements = metadata?.metadata?.requirements ?? []
  const statement = statements.find((item) => item.attributes.language === language)
  const ready = Boolean(
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
    if (!revision) return
    setBusy(true)
    setError('')
    try {
      const result = await api.postApiAuthoringProblemsIdChecks(problemId, {
        revision: Number(revision),
      })
      setRunningCheck(result)
      setChecks((current) => [result, ...current.filter((c) => c.id !== result.id)])
    } catch (e) {
      setError(apiError(e, '无法开始检查'))
    } finally {
      setBusy(false)
    }
  }
  async function publish() {
    if (!effectiveCheck || !ready) return
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
      setBusy(false)
    }
  }
  const checking =
    !!runningCheck &&
    runningCheck.dataHash === inspection?.dataHash &&
    ['queued', 'running'].includes(runningCheck.state)
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
          disabled={busy}
          onClick={() => setRefresh((value) => value + 1)}
        >
          <RefreshCw />
          刷新
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
                onChange={setRevision}
                options={commits.map((item) => [
                  String(item.revision),
                  `r${item.revision} · ${item.message}`,
                ])}
                disabled={busy || checking}
              />
              {statements.length > 1 && (
                <Choice
                  label="题面语言"
                  value={language}
                  onChange={(value) => {
                    setLanguage(value)
                    setPublished(undefined)
                  }}
                  options={statements.map((item) => [
                    item.attributes.language,
                    item.attributes.language === 'zh' ? '中文' : item.attributes.language,
                  ])}
                  disabled={busy}
                />
              )}
              <p className="text-xs text-muted-foreground">
                {inspection
                  ? `${inspection.testCount} 个测试 · ${inspection.sampleCount} 个样例`
                  : selectionError || '正在核对提交…'}
              </p>
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
                    <p className="text-sm font-medium">数据与程序检查通过</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {new Date(
                        effectiveCheck.finishedAt ?? effectiveCheck.createdAt,
                      ).toLocaleString()}
                      {statement?.attributes.format === 'tex'
                        ? ' · 将使用这次检查生成的 PDF 题面'
                        : ''}
                    </p>
                  </div>
                </div>
              ) : (
                <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-muted/35 p-4">
                  <div className="flex items-center gap-3">
                    <Circle className="size-4 text-muted-foreground" />
                    <p className="text-sm">{checking ? '正在运行检查…' : '此版本还没有通过检查'}</p>
                  </div>
                  {canCheck && (
                    <Button
                      size="sm"
                      variant="outline"
                      loading={checking || busy}
                      disabled={busy || checking || !inspection?.canBuild}
                      onClick={() => void checkSelected()}
                    >
                      <Play />
                      检查这个提交
                    </Button>
                  )}
                </div>
              )}
              {runningCheck?.state === 'failed' &&
                runningCheck.dataHash === inspection?.dataHash && (
                  <p role="alert" className="text-sm text-destructive">
                    {runningCheck.errorMessage || '检查未通过，请在运行检查页查看详细结果。'}
                  </p>
                )}
              {matches.length > 1 && (
                <details>
                  <summary className="cursor-pointer text-xs text-muted-foreground">
                    使用其他检查记录
                  </summary>
                  <div className="mt-3">
                    <Choice
                      label="成功检查"
                      value={effectiveCheck?.id ?? ''}
                      onChange={(v) => {
                        setCheckId(v)
                        setPublished(undefined)
                      }}
                      disabled={busy}
                      options={matches.map((item) => [
                        item.id,
                        new Date(item.finishedAt ?? item.createdAt).toLocaleString(),
                      ])}
                    />
                  </div>
                </details>
              )}
              {[
                ...(inspection?.issues.filter((i) => i.severity === 'error') ?? []),
                ...requirements,
                ...(inspection?.publicationIssues ?? []),
              ].map((item, index) => (
                <p key={index} className="text-sm text-amber-700 dark:text-amber-400">
                  {item.message}
                </p>
              ))}
              <Link
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                to={`/authoring/${problemId}/checks`}
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
                      ? `准备将 r${revision} 发布为 v${currentVersion + 1}`
                      : '完成上面的步骤后即可发布。'}
                </p>
                {canPublish ? (
                  <SaveButton
                    disabled={busy || copyBusy || !ready || !!releasedSelection}
                    loading={busy && ready}
                    saved={!!releasedSelection}
                    loadingLabel="发布中"
                    savedLabel={`已发布 v${releasedSelection?.version ?? ''}`}
                    onClick={() => void publish()}
                  >
                    发布{currentVersion ? ` v${currentVersion + 1}` : '首个版本'}
                  </SaveButton>
                ) : (
                  <span className="text-xs text-muted-foreground">需要发布权限</span>
                )}
              </div>
              {published && (
                <p role="status" className="text-sm text-primary">
                  v{published.version} 已发布。
                  <Link className="ml-2 underline" to={`/problems/${problemId}`}>
                    查看题目
                  </Link>
                </p>
              )}
            </section>
          </div>
          <aside className="space-y-5">
            <div className="rounded-xl border p-5">
              <p className="text-xs text-muted-foreground">题库当前使用</p>
              <p className="mt-2 text-3xl font-semibold tracking-tight">
                {currentVersion ? `v${currentVersion}` : '未发布'}
              </p>
              <p className="mt-3 text-xs leading-5 text-muted-foreground">
                发布版本保持不变。继续编辑草稿不会影响已经开赛的比赛。
              </p>
            </div>
            <section aria-label="发布历史">
              <h3 className="mb-3 text-xs font-medium text-muted-foreground">发布记录</h3>
              {releases.map((item) => (
                <div key={item.version} className="flex justify-between gap-3 border-l-2 py-2 pl-4">
                  <div>
                    <span className="text-sm font-medium">v{item.version}</span>
                    <p className="mt-1 text-xs text-muted-foreground">来自 r{item.revision}</p>
                  </div>
                  <time className="text-xs text-muted-foreground">
                    {new Date(item.createdAt).toLocaleDateString()}
                  </time>
                </div>
              ))}
              {!releases.length && <p className="text-xs text-muted-foreground">暂无发布记录</p>}
            </section>
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
              disabled={busy}
              onBusy={setCopyBusy}
            />
          </div>
        </details>
      )}
    </div>
  )
}
