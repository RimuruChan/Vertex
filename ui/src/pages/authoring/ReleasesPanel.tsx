import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
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
  onBusy,
}: {
  problemId: string
  canPublish: boolean
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI()
  const [copyBusy, setCopyBusy] = useState(false)
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
    !inspection.publicationIssues.length &&
    !requirements.length &&
    effectiveCheck &&
    statement?.attributes.format === 'markdown',
  )
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
  return (
    <div className="max-w-5xl space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">发布</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            从已提交的材料创建公开版本。比赛继续使用编排时固定的版本。
          </p>
        </div>
        <Button variant="outline" disabled={busy} onClick={() => setRefresh((value) => value + 1)}>
          <RefreshCw />
          刷新
        </Button>
      </header>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {!commits.length ? (
        <div className="rounded-xl border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">
            {loading ? '正在加载…' : '先提交工作副本，再选择提交进行检查和发布。'}
          </p>
          {!loading && (
            <Button asChild variant="outline" className="mt-4">
              <Link to={`/authoring/${problemId}/changes`}>查看更改与提交</Link>
            </Button>
          )}
        </div>
      ) : (
        <section className="rounded-xl border bg-card p-5 space-y-5" aria-label="发布版本">
          <div className="grid gap-4 md:grid-cols-2">
            <Choice
              label="发布提交"
              value={revision}
              onChange={setRevision}
              options={commits.map((item) => [
                String(item.revision),
                `r${item.revision} · ${item.message}`,
              ])}
              disabled={busy}
            />
            <Choice
              label="题面语言"
              value={language}
              onChange={(value) => {
                setLanguage(value)
                setPublished(undefined)
              }}
              options={statements.map((item) => [
                item.attributes.language,
                `${item.attributes.language} · ${item.attributes.format}`,
              ])}
              disabled={busy || !statements.length}
            />
          </div>
          {selectionError && (
            <p role="alert" className="text-sm text-destructive">
              {selectionError}
            </p>
          )}
          {!inspection && !selectionError && (
            <p className="text-sm text-muted-foreground">正在核对提交材料…</p>
          )}
          {inspection && (
            <>
              <div className="grid gap-4 border-y py-4 sm:grid-cols-3">
                <div>
                  <p className="text-xs text-muted-foreground">当前公开版本</p>
                  <p className="mt-1 font-medium">
                    {currentVersion ? `v${currentVersion}` : '尚未发布'}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">将采用的提交</p>
                  <p className="mt-1 font-medium">r{revision}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">测试数据</p>
                  <p className="mt-1 font-medium">
                    {inspection.testCount} 个测试 · {inspection.sampleCount} 个样例
                  </p>
                </div>
              </div>
              {matches.length > 0 ? (
                <Choice
                  label="采用的成功检查"
                  value={effectiveCheck?.id ?? ''}
                  onChange={(value) => {
                    setCheckId(value)
                    setPublished(undefined)
                  }}
                  disabled={busy}
                  options={matches.map((item) => [
                    item.id,
                    `${new Date(item.finishedAt ?? item.createdAt).toLocaleString()} · ${item.treeHash === inspection.treeHash ? '完整材料一致' : '评测材料一致'}`,
                  ])}
                />
              ) : (
                <p className="text-sm text-amber-700 dark:text-amber-400">
                  没有匹配的成功检查。请先对这份提交运行检查。
                </p>
              )}
              {inspection.issues
                .filter((item) => item.severity === 'error')
                .map((item, index) => (
                  <p key={index} className="text-sm text-destructive">
                    {item.message}
                  </p>
                ))}
              {requirements.map((item, index) => (
                <p key={index} className="text-sm text-amber-700 dark:text-amber-400">
                  {item.message}
                </p>
              ))}
              {inspection.publicationIssues.map((item, index) => (
                <p key={item.code + index} className="text-sm text-amber-700 dark:text-amber-400">
                  {item.message}
                </p>
              ))}
              {statement && statement.attributes.format !== 'markdown' && (
                <p className="text-sm text-amber-700 dark:text-amber-400">
                  此题面需要转换后才能公开展示，请先处理题面格式。
                </p>
              )}
              {!statement && (
                <p className="text-sm text-destructive">请选择此提交中存在的题面语言。</p>
              )}
            </>
          )}
          <div className="flex flex-wrap items-center gap-3">
            {canPublish ? (
              <SaveButton
                disabled={busy || copyBusy || !ready || Boolean(releasedSelection)}
                loading={busy}
                saved={Boolean(releasedSelection)}
                loadingLabel="发布中"
                savedLabel={`已发布 v${releasedSelection?.version ?? ''}`}
                onClick={() => void publish()}
              >
                {`发布${currentVersion ? ` v${currentVersion + 1}` : '首个版本'}`}
              </SaveButton>
            ) : (
              <p className="text-sm text-muted-foreground">
                你可以审阅发布材料，发布操作需要发布权限。
              </p>
            )}
            <Button variant="outline" asChild>
              <Link to={`/authoring/${problemId}/checks`}>查看检查</Link>
            </Button>
            {published && (
              <Button variant="outline" asChild>
                <Link to={`/problems/${problemId}`}>查看已发布题目</Link>
              </Button>
            )}
          </div>
          {published && (
            <p role="status" className="text-sm text-muted-foreground">
              v{published.version} 已发布，现有比赛的题目版本保持不变。
            </p>
          )}
        </section>
      )}
      <section className="overflow-hidden rounded-xl border" aria-label="发布历史">
        <h3 className="border-b px-5 py-3 text-sm font-medium">发布历史</h3>
        {!releases.length && (
          <p className="px-5 py-4 text-sm text-muted-foreground">尚无通过新工作台发布的版本。</p>
        )}
        {releases.map((item) => (
          <div
            key={item.version}
            className="flex flex-wrap items-center justify-between gap-2 border-b last:border-0 px-5 py-3 text-sm"
          >
            <p>
              <span className="mr-3 font-medium tabular-nums">v{item.version}</span>
              <span className="text-muted-foreground">
                r{item.revision} · {item.language}
              </span>
            </p>
            <span className="text-xs text-muted-foreground">
              {new Date(item.createdAt).toLocaleString()}
            </span>
          </div>
        ))}
      </section>
      <CopyReleasePanel
        problemId={problemId}
        releases={releases}
        disabled={busy}
        onBusy={setCopyBusy}
      />
    </div>
  )
}
