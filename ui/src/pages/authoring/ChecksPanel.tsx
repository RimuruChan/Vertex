import { Link } from '@/domain/navigation'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  ArrowUpRight,
  CheckCircle2,
  CircleAlert,
  Clock3,
  ListChecks,
  Loader2,
  Play,
  RefreshCw,
  Square,
} from 'lucide-react'
import type {
  DomainCheckRun,
  DomainContentCommit,
  DomainMaterialInspection,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Choice } from './MaterialForm'
import { validationModeName } from '@/lib/authoring-materials'
import { apiError } from '@/lib/format'
import { cn } from '@/lib/utils'
import VerdictTag from '@/components/VerdictTag'
import CheckStatements from './CheckStatements'
import { CheckIssueGroup } from './CheckPreparation'
import { checkMaterialTarget } from './check-material-target'

export const checkState = (state: string) =>
  ({
    queued: '排队中',
    running: '检查中',
    succeeded: '检查通过',
    failed: '检查失败',
    cancelled: '已取消',
  })[state] ?? state
const active = (item?: DomainCheckRun) => item?.state === 'queued' || item?.state === 'running'
const date = (value: string) => new Date(value).toLocaleString()
const stageLabel = (stage: string) =>
  ({
    queued: '等待执行',
    compile: '编译程序',
    generate: '生成数据',
    validate: '校验输入',
    answer: '准备答案',
    check: '核对答案',
    solutions: '检查参考解',
    'validation-tests': '校验器自测',
    statement: '编译题面',
    verifying: '检查参考解',
    package: '整理产物',
    done: '已完成',
  })[stage] ?? '执行中'

export default function ChecksPanel({
  problemId,
  copy,
  canEdit,
  revision,
  onBusy,
}: {
  problemId: string
  copy: DomainWorkingCopy
  canEdit: boolean
  revision?: number
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI()
  const [params, setParams] = useSearchParams()
  const source = (params.get('revision') ?? (revision ? String(revision) : 'copy')) || 'invalid'
  const requestedCheck = params.get('check') ?? ''
  const validSource =
    source === 'copy' || (/^[1-9]\d*$/.test(source) && Number.isSafeInteger(Number(source)))
  const [commits, setCommits] = useState<DomainContentCommit[]>([]),
    [inspectionResult, setInspectionResult] = useState<{
      key: string
      value: DomainMaterialInspection
    }>()
  const [checks, setChecks] = useState<DomainCheckRun[]>([]),
    [selected, setSelected] = useState(requestedCheck),
    [detail, setDetail] = useState<DomainCheckRun>()
  const [error, setError] = useState(''),
    [inspectionError, setInspectionError] = useState(''),
    [detailError, setDetailError] = useState('')
  const [busy, setBusy] = useState(false),
    [refresh, setRefresh] = useState(0),
    [loading, setLoading] = useState(true)
  const [testPage, setTestPage] = useState(0),
    [solutionPage, setSolutionPage] = useState(0),
    [dataPage, setDataPage] = useState(0),
    [validationPage, setValidationPage] = useState(0)
  const [onlyProblems, setOnlyProblems] = useState(false)
  const inspectionKey = JSON.stringify([source, copy.etag, refresh])
  const inspection = inspectionResult?.key === inspectionKey ? inspectionResult.value : undefined
  useEffect(() => {
    setSelected(requestedCheck || checks[0]?.id || '')
    setOnlyProblems(false)
    resetPages()
    // Polling must not reset the report the user is reading; only URL changes do.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [requestedCheck])
  function chooseSource(value: string) {
    setParams(
      (current) => {
        if (value === 'copy') current.delete('revision')
        else current.set('revision', value)
        current.delete('check')
        return current
      },
      { replace: true },
    )
  }
  function chooseCheck(id: string) {
    setSelected(id)
    setParams(
      (current) => {
        current.set('check', id)
        return current
      },
      { replace: true },
    )
  }
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    let live = true
    api
      .getApiAuthoringProblemsIdCommits(problemId, { limit: 100 })
      .then((value) => {
        if (live) setCommits(value.items)
      })
      .catch((error) => {
        if (live) setError(apiError(error, '提交列表加载失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId, copy.headRevision])
  useEffect(() => {
    let live = true
    setInspectionResult(undefined)
    setInspectionError('')
    if (!validSource) {
      setInspectionError('提交版本无效，请重新选择检查材料。')
      return
    }
    api
      .getApiAuthoringProblemsIdInspection(
        problemId,
        source === 'copy' ? { etag: copy.etag } : { revision: Number(source) },
      )
      .then((value) => {
        if (live) setInspectionResult({ key: inspectionKey, value })
      })
      .catch((error) => {
        if (live) setInspectionError(apiError(error, '材料检查失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId, source, copy.etag, refresh, validSource, inspectionKey])
  useEffect(() => {
    let live = true,
      timer: ReturnType<typeof setTimeout> | undefined
    const poll = async () => {
      try {
        const value = await api.getApiAuthoringProblemsIdChecks(problemId, { limit: 100 })
        if (!live) return
        setChecks(value.items)
        setSelected((current) => current || value.items[0]?.id || '')
        setError('')
        if (value.items.some(active)) timer = setTimeout(() => void poll(), 2500)
      } catch (error) {
        if (live) setError(apiError(error, '检查列表更新失败，可重试'))
      } finally {
        if (live) setLoading(false)
      }
    }
    void poll()
    return () => {
      live = false
      clearTimeout(timer)
    }
  }, [api, problemId, refresh])
  const selectedSummary = checks.find((item) => item.id === selected)
  useEffect(() => {
    if (!selected) return
    let live = true
    setDetailError('')
    api
      .getApiAuthoringProblemsIdChecksCheckId(problemId, selected)
      .then((value) => {
        if (live) setDetail(value)
      })
      .catch((error) => {
        if (live) setDetailError(apiError(error, '检查报告加载失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId, selected, selectedSummary, refresh])
  const report = detail?.id === selected ? detail : undefined
  async function start() {
    if (!canEdit || busy || startReason || !inspection) return
    setBusy(true)
    setError('')
    try {
      const value = await api.postApiAuthoringProblemsIdChecks(
        problemId,
        source === 'copy' ? { etag: copy.etag } : { revision: Number(source) },
      )
      setChecks((current) => [value, ...current.filter((item) => item.id !== value.id)])
      chooseCheck(value.id)
      setDetail(value)
      setOnlyProblems(false)
      resetPages()
      setRefresh((value) => value + 1)
    } catch (error) {
      setError(apiError(error, '无法开始检查'))
    } finally {
      setBusy(false)
    }
  }
  async function cancel() {
    if (!report) return
    setBusy(true)
    setError('')
    try {
      setDetail(await api.postApiAuthoringProblemsIdChecksCheckIdCancel(problemId, report.id))
      setRefresh((value) => value + 1)
    } catch (error) {
      setError(apiError(error, '取消失败'))
    } finally {
      setBusy(false)
    }
  }
  const sourceLabel =
    source === 'copy' ? '我的工作副本' : validSource ? `提交 r${source}` : '无效提交版本'
  const blockingIssues = inspection?.issues.filter((issue) => issue.severity === 'error') ?? []
  const warnings = inspection?.issues.filter((issue) => issue.severity !== 'error') ?? []
  const matchingRun = checks.find((item) => active(item) && item.treeHash === inspection?.treeHash)
  const startReason =
    source === 'copy' && copy.mergeId
      ? '先解决工作副本的合并冲突，再运行检查。'
      : inspectionError
        ? '材料信息未加载成功，请刷新重试。'
        : !inspection
          ? '正在确认材料是否完整…'
          : matchingRun
            ? '这份材料已有检查在运行，可在下方查看进度。'
            : !inspection.canBuild
              ? `先处理${blockingIssues.length ? ` ${blockingIssues.length} 项` : ''}阻塞问题，才能开始检查。`
              : ''
  const matchingTree = Boolean(inspection && report?.treeHash === inspection.treeHash)
  const matchingPolicy = Boolean(inspection && report?.policyVersion === inspection.policyVersion)
  const matchingData = Boolean(
    inspection && report?.dataHash === inspection.dataHash && matchingPolicy,
  )
  const solutionProblems = report?.solutions?.filter((item) => !item.matched) ?? []
  const validationProblems = report?.validation?.filter((item) => item.status !== 'ok') ?? []
  const dataProblems = report?.tests?.filter((item) => item.status !== 'ok') ?? []
  const problemCount = solutionProblems.length + validationProblems.length + dataProblems.length
  const visibleSolutions = onlyProblems ? solutionProblems : (report?.solutions ?? [])
  const visibleValidation = onlyProblems ? validationProblems : (report?.validation ?? [])
  const visibleData = onlyProblems ? dataProblems : (report?.tests ?? [])
  const pageCases = report?.tests?.slice(testPage * 20, testPage * 20 + 20) ?? []
  const pageSolutions = visibleSolutions.slice(solutionPage * 20, solutionPage * 20 + 20)
  function resetPages() {
    setTestPage(0)
    setSolutionPage(0)
    setDataPage(0)
    setValidationPage(0)
  }
  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">运行检查</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            运行参考解和校验器，确认数据、答案与题面可以交付。
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => setRefresh((value) => value + 1)}
          >
            <RefreshCw />
            刷新
          </Button>
        </div>
      </header>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <section className="rounded-xl border bg-card p-5 space-y-4" aria-label="检查选项">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="min-w-0 w-full max-w-lg">
            <Choice
              label="检查材料"
              value={source}
              onChange={chooseSource}
              disabled={busy}
              options={[
                ...(canEdit ? [['copy', '我的工作副本'] as [string, string]] : []),
                ...(source !== 'copy' && !commits.some((item) => String(item.revision) === source)
                  ? [[source, sourceLabel] as [string, string]]
                  : []),
                ...commits.map(
                  (item) =>
                    [String(item.revision), `r${item.revision} · ${item.message}`] as [
                      string,
                      string,
                    ],
                ),
              ]}
            />
            <p className="mt-2 text-xs leading-5 text-muted-foreground">
              {source === 'copy'
                ? '检查开始时会固定一份材料；之后的编辑需要重新检查。'
                : '检查已提交的固定版本。修改材料后，请选择工作副本重新检查。'}
            </p>
          </div>
          {canEdit && (
            <Button
              loading={busy}
              disabled={busy || Boolean(startReason)}
              aria-describedby={startReason ? 'check-start-reason' : undefined}
              onClick={() => void start()}
            >
              <Play />
              {matchingRun ? '检查正在进行' : '开始检查'}
            </Button>
          )}
        </div>
        {startReason && !inspectionError && (
          <p
            id="check-start-reason"
            className="flex items-center gap-2 rounded-lg bg-muted/50 px-3 py-2.5 text-xs leading-5 text-muted-foreground"
            role="status"
          >
            {matchingRun || !inspection ? (
              <Loader2 className="size-4 shrink-0 animate-spin" />
            ) : (
              <CircleAlert className="size-4 shrink-0" />
            )}
            {startReason}
            {matchingRun && selected !== matchingRun.id && (
              <button
                className="shrink-0 font-medium text-primary hover:underline"
                onClick={() => {
                  chooseCheck(matchingRun.id)
                  setOnlyProblems(false)
                  resetPages()
                }}
              >
                查看进度
              </button>
            )}
          </p>
        )}
        {inspectionError && (
          <p id="check-start-reason" role="alert" className="text-sm text-destructive">
            {inspectionError}
          </p>
        )}
        {inspection && (
          <>
            <div className="grid grid-cols-2 gap-3 border-t pt-4 sm:grid-cols-4">
              {[
                ['程序', inspection.programCount],
                ['测试点', inspection.testCount],
                ['其中样例', inspection.sampleCount],
                ['校验器自测', inspection.validationCount],
              ].map(([label, count]) => (
                <div key={label} className="rounded-lg bg-muted/40 px-3 py-2.5">
                  <p className="text-lg font-semibold tabular-nums">{count}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{label}</p>
                </div>
              ))}
            </div>
            {inspection.canBuild && !matchingRun && (
              <p className="flex items-center gap-2 text-xs text-emerald-700 dark:text-emerald-400">
                <CheckCircle2 className="size-4 shrink-0" />
                材料引用完整，可以开始检查
              </p>
            )}
            <CheckIssueGroup
              title="开始检查前需修复"
              description="这些问题会阻止程序与数据检查。"
              issues={blockingIssues}
              entries={copy.tree.entries}
              problemId={problemId}
              historical={source !== 'copy'}
              canEdit={canEdit}
              blocking
            />
            <CheckIssueGroup
              title="材料提醒"
              description="可以继续检查，建议一并确认。"
              issues={warnings}
              entries={copy.tree.entries}
              problemId={problemId}
              historical={source !== 'copy'}
              canEdit={canEdit}
            />
            <CheckIssueGroup
              title="发布前需处理"
              description="不影响运行检查，但会影响创建发布版本。"
              issues={inspection.publicationIssues}
              entries={copy.tree.entries}
              problemId={problemId}
              historical={source !== 'copy'}
              canEdit={canEdit}
            />
          </>
        )}
      </section>
      {report?.state === 'succeeded' && matchingData && inspection?.canBuild && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
          <div className="flex items-center gap-3">
            <CheckCircle2 className="size-5 text-primary" />
            <div>
              <p className="text-sm font-medium">{sourceLabel}的数据和程序已通过检查</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {!matchingTree ? '题面等内容已变化，需核对当前排版。' : ''}
                {inspection?.publicationIssues.length
                  ? '仍有发布前需处理的问题，请先查看上方提醒。'
                  : source !== 'copy' ||
                      (matchingTree && (report.revision || report.matchingRevision))
                    ? '接下来确认对外发布的版本。'
                    : '先记录一次提交，再创建发布版本。'}
              </p>
            </div>
          </div>
          <Button variant="outline" asChild>
            <Link
              to={`/authoring/${problemId}/${source !== 'copy' || (matchingTree && (report.revision || report.matchingRevision)) ? 'releases' : 'changes'}`}
            >
              {source !== 'copy' || (matchingTree && (report.revision || report.matchingRevision))
                ? '前往发布'
                : '提交这份草稿'}
            </Link>
          </Button>
        </div>
      )}
      <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[250px_minmax(0,1fr)]">
        <section aria-label="检查记录" className="min-w-0 rounded-xl border overflow-hidden">
          <h3 className="flex items-center justify-between border-b px-4 py-3 text-xs font-medium text-muted-foreground">
            <span>最近的检查</span>
            <span className="tabular-nums">{checks.length}</span>
          </h3>
          {!checks.length && (
            <p className="p-4 text-sm text-muted-foreground">
              {loading ? '正在加载…' : '尚无检查记录'}
            </p>
          )}
          <div className="max-h-[32rem] overflow-auto">
            {checks.map((item) => (
              <button
                key={item.id}
                onClick={() => {
                  chooseCheck(item.id)
                  setOnlyProblems(false)
                  resetPages()
                }}
                aria-pressed={selected === item.id}
                className={cn(
                  'block w-full border-b border-l-2 border-l-transparent last:border-b-0 px-4 py-3 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
                  selected === item.id && 'border-l-primary bg-primary/5',
                )}
              >
                <span className="flex items-center justify-between gap-2 text-sm">
                  <span className="inline-flex items-center gap-2 font-medium">
                    {active(item) ? (
                      <Loader2 className="size-3.5 animate-spin text-primary" />
                    ) : item.state === 'succeeded' ? (
                      <CheckCircle2 className="size-3.5 text-emerald-600 dark:text-emerald-400" />
                    ) : item.state === 'failed' ? (
                      <CircleAlert className="size-3.5 text-destructive" />
                    ) : (
                      <Clock3 className="size-3.5 text-muted-foreground" />
                    )}
                    {checkState(item.state)}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {item.revision || item.matchingRevision
                      ? `r${item.revision || item.matchingRevision}`
                      : '工作副本'}
                  </span>
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  {date(item.createdAt)}
                </span>
                {inspection && (
                  <span
                    className={cn(
                      'mt-2 inline-block rounded px-1.5 py-0.5 text-[11px]',
                      item.treeHash === inspection.treeHash &&
                        item.policyVersion === inspection.policyVersion
                        ? 'bg-primary/10 text-primary'
                        : 'bg-muted text-muted-foreground',
                    )}
                  >
                    {item.policyVersion !== inspection.policyVersion
                      ? '检查规则已变化'
                      : item.treeHash === inspection.treeHash
                        ? '匹配所选材料'
                        : item.dataHash === inspection.dataHash
                          ? '数据与程序匹配'
                          : '其他版本的检查'}
                  </span>
                )}
              </button>
            ))}
          </div>
        </section>
        <section aria-label="检查报告" className="min-w-0 space-y-4">
          {!selected && !loading && (
            <div className="flex min-h-64 flex-col items-center justify-center rounded-xl border border-dashed p-6 text-center">
              <ListChecks className="mb-3 size-8 text-muted-foreground/60" />
              <h3 className="text-sm font-medium">开始第一次检查</h3>
              <p className="mt-2 max-w-sm text-xs leading-6 text-muted-foreground">
                选择上方材料并开始检查。这里会展示参考解、测试数据和校验器的结果，帮助你定位需要处理的问题。
              </p>
            </div>
          )}
          {detailError && (
            <p role="alert" className="text-sm text-destructive">
              {detailError}
            </p>
          )}
          {selected && !report && !detailError && (
            <p className="text-sm text-muted-foreground">正在加载报告…</p>
          )}
          {report && (
            <>
              <div className="rounded-xl border bg-card p-5 space-y-3">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <h3 className="flex items-center gap-2 font-medium">
                      {report.state === 'succeeded' ? (
                        <CheckCircle2 className="size-5 text-emerald-600 dark:text-emerald-400" />
                      ) : active(report) ? (
                        <Loader2 className="size-5 animate-spin text-primary" />
                      ) : report.state === 'failed' ? (
                        <CircleAlert className="size-5 text-destructive" />
                      ) : (
                        <Clock3 className="size-5 text-muted-foreground" />
                      )}
                      {checkState(report.state)}
                    </h3>
                    <p className="mt-1.5 text-xs text-muted-foreground">
                      {report.revision || report.matchingRevision
                        ? `提交 r${report.revision || report.matchingRevision}`
                        : '工作副本快照'}{' '}
                      · {date(report.createdAt)}
                    </p>
                  </div>
                  {canEdit && active(report) && (
                    <Button variant="outline" disabled={busy} onClick={() => void cancel()}>
                      <Square />
                      取消检查
                    </Button>
                  )}
                </div>
                {inspection && (
                  <div
                    className={cn(
                      'rounded-lg px-3 py-2.5 text-xs leading-5',
                      matchingTree && matchingPolicy
                        ? 'bg-muted/50 text-muted-foreground'
                        : 'bg-amber-500/10 text-amber-800 dark:text-amber-300',
                    )}
                  >
                    {matchingTree && matchingPolicy
                      ? `这份报告与所选的${sourceLabel}一致。`
                      : matchingData
                        ? '数据与程序一致，题面等内容已变化。报告中的题面预览对应检查时的版本。'
                        : !matchingPolicy
                          ? '检查规则已有变化，这份报告不能作为当前材料通过检查的依据。请重新检查。'
                          : `这份报告对应其他材料，未覆盖所选的${sourceLabel}。请重新检查确认。`}
                  </div>
                )}
                {active(report) && (
                  <>
                    <progress
                      className="w-full h-1.5 accent-primary"
                      aria-label="检查进度"
                      max={Math.max(1, report.progressTotal)}
                      value={report.progressDone}
                    />
                    <p className="text-xs text-muted-foreground">
                      {stageLabel(report.stage)} · {report.progressDone} /{' '}
                      {report.progressTotal || '—'}
                    </p>
                  </>
                )}
                {report.errorMessage && (
                  <p
                    role="alert"
                    className="whitespace-pre-wrap break-words text-sm text-destructive"
                  >
                    {report.errorMessage}
                  </p>
                )}
              </div>
              {Boolean(
                report.solutions?.length || report.tests?.length || report.validation?.length,
              ) && (
                <div className="space-y-3">
                  <div className="grid grid-cols-3 gap-2">
                    {[
                      ['参考解', report.solutions?.length ?? 0, solutionProblems.length],
                      ['测试数据', report.tests?.length ?? 0, dataProblems.length],
                      ['校验器自测', report.validation?.length ?? 0, validationProblems.length],
                    ].map(([label, total, problems]) => (
                      <div key={label} className="rounded-lg border bg-card p-3">
                        <p className="text-xs text-muted-foreground">{label}</p>
                        <p
                          className={cn(
                            'mt-1.5 text-sm font-semibold tabular-nums',
                            problems ? 'text-destructive' : 'text-foreground',
                          )}
                        >
                          {problems
                            ? `${problems} 项异常`
                            : `${total} 项${total ? '符合预期' : '结果'}`}
                        </p>
                      </div>
                    ))}
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="text-xs text-muted-foreground">
                      {active(report)
                        ? '结果会随检查进度持续更新'
                        : '按预期判定结果，非通过的参考解也可能符合预期'}
                    </p>
                    <div
                      className="flex gap-1 rounded-lg bg-muted/50 p-1"
                      aria-label="筛选检查结果"
                    >
                      {[false, true].map((problems) => (
                        <Button
                          key={String(problems)}
                          size="sm"
                          variant={onlyProblems === problems ? 'secondary' : 'ghost'}
                          aria-pressed={onlyProblems === problems}
                          onClick={() => {
                            setOnlyProblems(problems)
                            resetPages()
                          }}
                        >
                          {problems ? `只看异常 (${problemCount})` : '全部结果'}
                        </Button>
                      ))}
                    </div>
                  </div>
                  {onlyProblems && !problemCount && (
                    <p className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">
                      {active(report)
                        ? '已返回的结果暂无异常，检查仍在运行。'
                        : report.state === 'failed'
                          ? '逐项结果中没有异常，请查看上方错误信息和执行日志。'
                          : '已返回的逐项结果均符合预期。'}
                    </p>
                  )}
                </div>
              )}
              {!onlyProblems && Boolean(report.statements?.length) && (
                <CheckStatements
                  key={report.id}
                  problemId={problemId}
                  checkId={report.id}
                  statements={report.statements!}
                />
              )}
              {Boolean(visibleSolutions.length) && (
                <div className="rounded-xl border overflow-hidden">
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
                    <h3 className="text-sm font-medium">参考解结果</h3>
                    <span className="text-xs text-muted-foreground">
                      {report.solutions?.filter((item) => item.matched).length} /{' '}
                      {report.solutions?.length} 符合预期
                    </span>
                  </div>
                  <div className="overflow-auto">
                    <table className="w-full text-sm">
                      <thead className="bg-muted/30 text-muted-foreground">
                        <tr>
                          <th className="px-4 py-2 text-left font-normal">程序 / 预期</th>
                          <th className="px-3 py-2 font-normal">结果</th>
                          {pageCases.map((item) => (
                            <th key={item.index} className="px-3 py-2 font-normal">
                              #{item.index}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {pageSolutions.map((solution, index) => (
                          <tr key={index} className="border-t">
                            <td className="px-4 py-3">
                              <span className="block min-w-32 font-medium">{solution.name}</span>
                              <span className="text-xs text-muted-foreground">
                                预期：{solution.expectedVerdict || '未指定'}
                              </span>
                              {Boolean(solution.failedTest) && (
                                <p className="mt-1 text-xs text-destructive">
                                  首个未通过的测试：#{solution.failedTest}
                                </p>
                              )}
                              {solution.message && (
                                <details className="mt-2 text-xs">
                                  <summary className="cursor-pointer text-muted-foreground">
                                    诊断信息
                                  </summary>
                                  <pre className="mt-2 max-w-lg whitespace-pre-wrap break-words font-sans leading-5">
                                    {solution.message}
                                  </pre>
                                </details>
                              )}
                            </td>
                            <td
                              className={cn(
                                'whitespace-nowrap px-3 py-3 text-center',
                                !solution.matched && 'text-destructive',
                              )}
                            >
                              <VerdictTag status={solution.actualVerdict} />
                              <p className="mt-1 text-[11px]">
                                {solution.matched ? '符合预期' : '不符合预期'}
                              </p>
                            </td>
                            {pageCases.map((item) => {
                              const result = solution.cases?.find(
                                (result) => result.index === item.index,
                              )
                              return (
                                <td
                                  key={item.index}
                                  className="px-3 py-3 text-center text-xs whitespace-nowrap"
                                  title={
                                    result
                                      ? `${result.timeMs} ms · ${result.memoryKb} KiB`
                                      : undefined
                                  }
                                >
                                  {result ? <VerdictTag status={result.verdict} /> : '—'}
                                </td>
                              )
                            })}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  {(visibleSolutions.length > 20 || (report.tests?.length ?? 0) > 20) && (
                    <div className="flex flex-wrap justify-between gap-3 border-t p-3">
                      <div className="flex items-center gap-2 text-xs">
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={solutionPage === 0}
                          onClick={() => setSolutionPage((value) => value - 1)}
                        >
                          上一组程序
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={(solutionPage + 1) * 20 >= visibleSolutions.length}
                          onClick={() => setSolutionPage((value) => value + 1)}
                        >
                          下一组程序
                        </Button>
                      </div>
                      <div className="flex items-center gap-2 text-xs">
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={testPage === 0}
                          onClick={() => setTestPage((value) => value - 1)}
                        >
                          上一组测试
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={(testPage + 1) * 20 >= (report.tests?.length ?? 0)}
                          onClick={() => setTestPage((value) => value + 1)}
                        >
                          下一组测试
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
              )}
              {Boolean(visibleValidation.length) && (
                <section className="space-y-3 rounded-xl border p-4" aria-label="校验器自测结果">
                  <h3 className="text-sm font-medium">
                    校验器自测 · {visibleValidation.length} 项{onlyProblems ? '异常' : ''}
                  </h3>
                  <div className="divide-y">
                    {visibleValidation
                      .slice(validationPage * 25, validationPage * 25 + 25)
                      .map((item) => {
                        const entry = copy.tree.entries.find(
                          (entry) => entry.id === item.id && entry.kind === 'validation',
                        )
                        const target =
                          canEdit && entry ? checkMaterialTarget(problemId, entry) : undefined
                        return (
                          <div key={item.id} className="flex items-start gap-3 py-3">
                            {item.status === 'ok' ? (
                              <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                            ) : (
                              <CircleAlert className="mt-0.5 size-4 shrink-0 text-destructive" />
                            )}
                            <div className="min-w-0 flex-1">
                              <p className="break-words text-sm font-medium">{item.name}</p>
                              <p className="mt-1 text-xs text-muted-foreground">
                                {validationModeName(item.mode)} ·{' '}
                                {(
                                  {
                                    accepted: '实际接受',
                                    rejected: '实际拒绝',
                                    error: '执行异常',
                                    invalid_input: '输入校验失败',
                                  } as Record<string, string>
                                )[item.actual] ?? item.actual}{' '}
                                · {item.status === 'ok' ? '符合预期' : '不符合预期'}
                              </p>
                              {item.message && (
                                <p className="mt-2 whitespace-pre-wrap break-words text-xs text-muted-foreground">
                                  {item.message}
                                </p>
                              )}
                            </div>
                            {target && (
                              <Link
                                to={target}
                                className="inline-flex shrink-0 items-center gap-1 rounded px-2 py-1 text-xs text-primary hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                              >
                                {source === 'copy' && matchingTree
                                  ? item.status === 'ok'
                                    ? '查看材料'
                                    : '去修复'
                                  : '查看当前材料'}
                                <ArrowUpRight className="size-3.5" />
                              </Link>
                            )}
                          </div>
                        )
                      })}
                  </div>
                  {visibleValidation.length > 25 && (
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={validationPage === 0}
                        onClick={() => setValidationPage((value) => value - 1)}
                      >
                        上一页
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={(validationPage + 1) * 25 >= visibleValidation.length}
                        onClick={() => setValidationPage((value) => value + 1)}
                      >
                        下一页
                      </Button>
                    </div>
                  )}
                </section>
              )}
              {Boolean(visibleData.length) && (
                <details
                  key={`${report.id}-${onlyProblems}`}
                  open={onlyProblems || undefined}
                  className="rounded-xl border p-4"
                >
                  <summary className="cursor-pointer text-sm font-medium">
                    测试数据 · {visibleData.length} 个{onlyProblems ? '异常' : '测试'}
                  </summary>
                  <div className="mt-3 max-h-80 space-y-2 overflow-auto">
                    {visibleData.slice(dataPage * 50, dataPage * 50 + 50).map((item) => (
                      <div key={item.index} className="border-t pt-2 text-sm">
                        <p className="flex items-center gap-2">
                          {item.status === 'ok' ? (
                            <CheckCircle2 className="size-3.5 text-emerald-600 dark:text-emerald-400" />
                          ) : (
                            <CircleAlert className="size-3.5 text-destructive" />
                          )}
                          #{item.index} {item.isSample ? '样例' : '秘密数据'} ·{' '}
                          {item.status === 'ok' ? '通过' : item.status}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          输入 {item.inputBytes} B · 答案 {item.answerBytes} B
                        </p>
                        {item.message && (
                          <p className="mt-1 whitespace-pre-wrap break-words text-xs leading-5">
                            {item.message}
                          </p>
                        )}
                        {(item.inputHead || item.answerHead || item.command) && (
                          <details className="mt-2 mb-2">
                            <summary className="cursor-pointer text-xs text-primary">
                              查看本次输入与答案
                            </summary>
                            {item.command && (
                              <pre className="mt-2 rounded bg-muted/50 p-2 text-xs whitespace-pre-wrap break-words">
                                {item.command}
                              </pre>
                            )}
                            <div className="mt-2 grid gap-2 sm:grid-cols-2">
                              {(
                                [
                                  ['输入', item.inputHead],
                                  ['答案', item.answerHead],
                                ] as const
                              ).map(([label, value]) => (
                                <div key={label} className="min-w-0 rounded border bg-muted/30 p-2">
                                  <p className="mb-1 text-[11px] text-muted-foreground">{label}</p>
                                  <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words text-xs">
                                    {value || '未返回内容预览'}
                                  </pre>
                                </div>
                              ))}
                            </div>
                            {item.headsTruncated && (
                              <p className="mt-1 text-[11px] text-muted-foreground">
                                这里只展示文件开头的部分内容。
                              </p>
                            )}
                          </details>
                        )}
                      </div>
                    ))}
                  </div>
                  {visibleData.length > 50 && (
                    <div className="mt-3 flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={dataPage === 0}
                        onClick={() => setDataPage((value) => value - 1)}
                      >
                        上一页数据
                      </Button>
                      <span className="text-xs text-muted-foreground">第 {dataPage + 1} 页</span>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={(dataPage + 1) * 50 >= visibleData.length}
                        onClick={() => setDataPage((value) => value + 1)}
                      >
                        下一页数据
                      </Button>
                    </div>
                  )}
                </details>
              )}
              {report.log && (
                <details className="rounded-xl border p-4">
                  <summary className="cursor-pointer text-sm font-medium">执行日志</summary>
                  <pre className="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-words text-xs leading-5">
                    {report.log}
                  </pre>
                </details>
              )}
            </>
          )}
        </section>
      </div>
    </div>
  )
}
