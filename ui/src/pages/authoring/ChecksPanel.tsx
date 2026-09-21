import { Link } from '@/domain/navigation'
import { useEffect, useState } from 'react'
import { CheckCircle2, CircleAlert, Play, RefreshCw, Square } from 'lucide-react'
import type {
  DomainCheckRun,
  DomainContentCommit,
  DomainMaterialInspection,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Choice } from './MaterialForm'
import { validationModeName, entryLabel } from '@/lib/authoring-materials'
import { apiError } from '@/lib/format'
import { cn } from '@/lib/utils'
import VerdictTag from '@/components/VerdictTag'
import CheckStatements from './CheckStatements'

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
  const [source, setSource] = useState(revision ? String(revision) : 'copy')
  const [commits, setCommits] = useState<DomainContentCommit[]>([]),
    [inspection, setInspection] = useState<DomainMaterialInspection>()
  const [checks, setChecks] = useState<DomainCheckRun[]>([]),
    [selected, setSelected] = useState(''),
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
    setInspection(undefined)
    setInspectionError('')
    api
      .getApiAuthoringProblemsIdInspection(
        problemId,
        source === 'copy' ? { etag: copy.etag } : { revision: Number(source) },
      )
      .then((value) => {
        if (live) setInspection(value)
      })
      .catch((error) => {
        if (live) setInspectionError(apiError(error, '材料检查失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId, source, copy.etag])
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
    setBusy(true)
    setError('')
    try {
      const value = await api.postApiAuthoringProblemsIdChecks(
        problemId,
        source === 'copy' ? { etag: copy.etag } : { revision: Number(source) },
      )
      setChecks((current) => [value, ...current.filter((item) => item.id !== value.id)])
      setSelected(value.id)
      setDetail(value)
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
  const pageCases = report?.tests?.slice(testPage * 20, testPage * 20 + 20) ?? []
  const pageSolutions = report?.solutions?.slice(solutionPage * 20, solutionPage * 20 + 20) ?? []
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
          </Button>{' '}
          {canEdit && (
            <Button
              loading={busy}
              disabled={
                busy ||
                !inspection?.canBuild ||
                checks.some((item) => active(item) && item.treeHash === inspection?.treeHash) ||
                (source === 'copy' && Boolean(copy.mergeId))
              }
              onClick={() => void start()}
            >
              <Play />
              开始检查
            </Button>
          )}
        </div>
      </header>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <section className="rounded-xl border bg-card p-5 space-y-4" aria-label="检查选项">
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-0 w-full max-w-lg">
            <Choice
              label="检查材料"
              value={source}
              onChange={setSource}
              disabled={busy}
              options={[
                ...(canEdit ? [['copy', '我的工作副本'] as [string, string]] : []),
                ...commits.map(
                  (item) =>
                    [String(item.revision), `r${item.revision} · ${item.message}`] as [
                      string,
                      string,
                    ],
                ),
              ]}
            />
          </div>
        </div>
        {inspectionError && (
          <p role="alert" className="text-sm text-destructive">
            {inspectionError}
          </p>
        )}
        {!inspection && !inspectionError && (
          <p className="text-sm text-muted-foreground">正在检查材料引用…</p>
        )}
        {inspection && (
          <>
            <p className="text-sm text-muted-foreground">
              {inspection.programCount} 个程序 · {inspection.testCount} 个测试 ·{' '}
              {inspection.validationCount} 项校验器自测 · {inspection.sampleCount} 个样例
              {inspection.canBuild ? ' · 可以开始检查' : ''}
            </p>
            {inspection.issues.length > 0 && (
              <ul className="max-h-64 space-y-2 overflow-auto">
                {inspection.issues.map((issue, index) => (
                  <li key={index} className="flex items-start gap-2 text-sm">
                    <CircleAlert
                      className={cn(
                        'mt-0.5 size-4 shrink-0',
                        issue.severity === 'error'
                          ? 'text-destructive'
                          : 'text-amber-600 dark:text-amber-400',
                      )}
                    />
                    <div>
                      <p>{issue.message}</p>
                      {issue.entryId && (
                        <p className="text-xs text-muted-foreground">
                          {(() => {
                            const item = copy.tree.entries.find(
                              (entry) => entry.id === issue.entryId,
                            )
                            return item ? entryLabel(item) : '已移除的材料'
                          })()}
                        </p>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
            {inspection.publicationIssues.length > 0 && (
              <div className="space-y-1 border-t pt-3">
                <p className="text-xs font-medium">可以检查材料，发布前还需处理：</p>
                {inspection.publicationIssues.map((issue, index) => (
                  <p
                    key={issue.code + index}
                    className="text-xs text-amber-700 dark:text-amber-400"
                  >
                    {issue.message}
                  </p>
                ))}
              </div>
            )}
          </>
        )}
      </section>
      {report?.state === 'succeeded' && report.dataHash === inspection?.dataHash && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
          <div className="flex items-center gap-3">
            <CheckCircle2 className="size-5 text-primary" />
            <div>
              <p className="text-sm font-medium">数据和程序已通过检查</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {report.revision || report.matchingRevision
                  ? '接下来确认对外发布的版本。'
                  : '先记录一次提交，再创建发布版本。'}
              </p>
            </div>
          </div>
          <Button variant="outline" asChild>
            <Link
              to={`/authoring/${problemId}/${report.revision || report.matchingRevision ? 'releases' : 'changes'}`}
            >
              {report.revision || report.matchingRevision ? '前往发布' : '提交这份草稿'}
            </Link>
          </Button>
        </div>
      )}
      <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[240px_minmax(0,1fr)]">
        <section aria-label="检查记录" className="min-w-0 rounded-xl border overflow-hidden">
          <h3 className="border-b px-4 py-3 text-xs font-medium text-muted-foreground">
            最近的检查
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
                  setSelected(item.id)
                  setTestPage(0)
                  setSolutionPage(0)
                  setDataPage(0)
                  setValidationPage(0)
                }}
                aria-pressed={selected === item.id}
                className={cn(
                  'block w-full border-b last:border-0 px-4 py-3 text-left transition-colors hover:bg-muted/50',
                  selected === item.id && 'bg-muted/70',
                )}
              >
                <span className="flex items-center justify-between gap-2 text-sm">
                  <span>{checkState(item.state)}</span>
                  <span className="text-xs text-muted-foreground">
                    {item.revision || item.matchingRevision
                      ? `r${item.revision || item.matchingRevision}`
                      : '私人副本'}
                  </span>
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  {date(item.createdAt)}
                </span>
              </button>
            ))}
          </div>
        </section>
        <section aria-label="检查报告" className="min-w-0 space-y-4">
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
                  <h3 className="flex items-center gap-2 font-medium">
                    {report.state === 'succeeded' && (
                      <CheckCircle2 className="size-5 text-emerald-600 dark:text-emerald-400" />
                    )}
                    {checkState(report.state)}
                  </h3>
                  {canEdit && active(report) && (
                    <Button variant="outline" disabled={busy} onClick={() => void cancel()}>
                      <Square />
                      取消检查
                    </Button>
                  )}
                </div>
                <p className="text-sm text-muted-foreground">
                  {report.treeHash === inspection?.treeHash
                    ? '与当前选择的材料一致'
                    : report.dataHash === inspection?.dataHash
                      ? '评测材料一致，题面等内容存在更改'
                      : '检查针对之前固定的材料'}
                </p>
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
              {Boolean(report.statements?.length) && (
                <CheckStatements
                  key={report.id}
                  problemId={problemId}
                  checkId={report.id}
                  statements={report.statements!}
                />
              )}
              {Boolean(report.solutions?.length) && (
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
                                {solution.expectedVerdict}
                              </span>
                            </td>
                            <td
                              className={cn(
                                'whitespace-nowrap px-3 py-3 text-center',
                                !solution.matched && 'text-destructive',
                              )}
                            >
                              <VerdictTag status={solution.actualVerdict} />
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
                  {((report.solutions?.length ?? 0) > 20 || (report.tests?.length ?? 0) > 20) && (
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
                          disabled={(solutionPage + 1) * 20 >= (report.solutions?.length ?? 0)}
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
              {Boolean(report.validation?.length) && (
                <section className="space-y-3 rounded-xl border p-4" aria-label="校验器自测结果">
                  <h3 className="text-sm font-medium">
                    校验器自测 · {report.validation?.length} 项
                  </h3>
                  <div className="divide-y">
                    {report.validation
                      ?.slice(validationPage * 25, validationPage * 25 + 25)
                      .map((item) => (
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
                        </div>
                      ))}
                  </div>
                  {(report.validation?.length ?? 0) > 25 && (
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
                        disabled={(validationPage + 1) * 25 >= (report.validation?.length ?? 0)}
                        onClick={() => setValidationPage((value) => value + 1)}
                      >
                        下一页
                      </Button>
                    </div>
                  )}
                </section>
              )}
              {Boolean(report.tests?.length) && (
                <details className="rounded-xl border p-4">
                  <summary className="cursor-pointer text-sm font-medium">
                    测试数据 · {report.tests?.length} 个测试
                  </summary>
                  <div className="mt-3 max-h-80 space-y-2 overflow-auto">
                    {report.tests?.slice(dataPage * 50, dataPage * 50 + 50).map((item) => (
                      <div key={item.index} className="border-t pt-2 text-sm">
                        <p>
                          #{item.index} {item.isSample ? '样例' : '秘密数据'} · {item.status}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          输入 {item.inputBytes} B · 答案 {item.answerBytes} B
                        </p>
                        {item.message && (
                          <p className="whitespace-pre-wrap break-words text-xs">{item.message}</p>
                        )}
                      </div>
                    ))}
                  </div>
                  {(report.tests?.length ?? 0) > 50 && (
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
                        disabled={(dataPage + 1) * 50 >= (report.tests?.length ?? 0)}
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
