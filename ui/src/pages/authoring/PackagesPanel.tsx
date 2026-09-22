import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Archive,
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  Download,
  FileArchive,
  RefreshCw,
  Upload,
} from 'lucide-react'
import { Link } from '@/domain/navigation'
import type {
  DomainContentTree,
  DomainImportReceipt,
  DomainPackageExport,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Choice, Field } from './MaterialForm'
import { apiError, formatFileSize } from '@/lib/format'
import { entryLabel, materialNames } from '@/lib/authoring-materials'
import { cn } from '@/lib/utils'
import { importReviewProblem, packageImpact, packageSourceKey } from './package-review'
import PackageIssues from './PackageIssues'

const formatLabels: Record<string, string> = {
  vertex: 'Vertex 原生归档',
  'luogu-data': '洛谷数据 ZIP',
  'kattis-legacy-icpc': 'ICPC legacy-icpc',
  'kattis-legacy': 'Kattis legacy',
  'kattis-2025-09': 'ICPC / Kattis 2025-09',
  domjudge: 'DOMjudge',
  polygon: 'Polygon',
}
type ImportReview = {
  receipt: DomainImportReceipt
  before: DomainContentTree
  file: File
  timeLimit: string
}
type ExportReview = {
  value: DomainPackageExport
  sourceKey: string
  sourceLabel: string
  createdAt: string
  format: string
}

export default function PackagesPanel({
  problemId,
  copy,
  canEdit,
  revision,
  onSaved,
  onBusy,
}: {
  problemId: string
  copy: DomainWorkingCopy
  canEdit: boolean
  revision?: number
  onSaved: (copy: DomainWorkingCopy) => void
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI()
  const [mode, setMode] = useState<'import' | 'export'>(canEdit ? 'import' : 'export')
  const [file, setFile] = useState<File>(),
    [timeLimit, setTimeLimit] = useState('')
  const [review, setReview] = useState<ImportReview>(),
    [exported, setExported] = useState<ExportReview>()
  const [format, setFormat] = useState('vertex'),
    [busy, setBusy] = useState(''),
    [importError, setImportError] = useState(''),
    [exportError, setExportError] = useState(''),
    [applied, setApplied] = useState(false)
  const upload = useRef<HTMLInputElement>(null)
  const operation = useRef(false)
  const appliedReceipt = useRef('')
  const [now, setNow] = useState(Date.now)
  const sourceKey = packageSourceKey(copy.etag, revision)
  const sourceLabel = revision ? `提交 r${revision}` : '当前已保存的工作副本'
  const latest = useRef({ copy, canEdit, file, timeLimit, sourceKey, format })
  latest.current = { copy, canEdit, file, timeLimit, sourceKey, format }
  const receipt = review?.receipt
  const parametersMatch = Boolean(
    review && review.file === file && review.timeLimit === timeLimit.trim(),
  )
  const reviewProblem = receipt
    ? importReviewProblem({
        receipt,
        parametersMatch,
        etag: copy.etag,
        mergeId: copy.mergeId,
        canEdit,
        now,
      })
    : undefined
  const impact = useMemo(
    () => (review ? packageImpact(review.before, review.receipt.plan.tree) : undefined),
    [review],
  )
  const exportStale = Boolean(
    exported && (exported.sourceKey !== sourceKey || exported.format !== format),
  )
  const [impactFilter, setImpactFilter] = useState<'added' | 'modified' | 'removed'>('added')
  const [impactQuery, setImpactQuery] = useState('')
  const demo = import.meta.env.VITE_MOCK === 'true'
  useEffect(() => {
    onBusy(Boolean(busy))
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    if (!receipt || applied) return
    setNow(Date.now())
    const remaining = Date.parse(receipt.expiresAt) - Date.now()
    if (!Number.isFinite(remaining) || remaining <= 0) return
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(remaining + 1, 2147483647))
    return () => window.clearTimeout(timer)
  }, [receipt, applied])
  function chooseFile(chosen: File) {
    if (operation.current || !canEdit) return
    setFile(chosen)
    setApplied(false)
    setImportError('')
  }
  async function preview(chosen = file) {
    if (!chosen || operation.current || !latest.current.canEdit || latest.current.copy.mergeId)
      return
    setImportError('')
    setApplied(false)
    const limit = timeLimit.trim() ? Number(timeLimit) : undefined
    if (limit !== undefined && (!Number.isInteger(limit) || limit < 1 || limit > 3600000)) {
      setImportError('时间限制应为 1–3600000 的整数毫秒。')
      return
    }
    if (chosen.size > (demo ? 8 : 64) * 1024 * 1024) {
      setImportError(
        demo ? '演示题包最大 8 MiB；大题包请使用真实后端。' : '题包超过 64 MiB，请缩小归档后重试。',
      )
      return
    }
    operation.current = true
    setBusy('preview')
    setReview(undefined)
    const base = latest.current.copy
    const before = structuredClone(base.tree)
    try {
      const receipt = await api.postApiAuthoringProblemsIdImports(problemId, {
        file: chosen,
        etag: base.etag,
        timeLimitMs: limit,
      })
      const next = {
        receipt,
        before,
        file: chosen,
        timeLimit: timeLimit.trim(),
      }
      setReview(next)
      const changes = packageImpact(next.before, receipt.plan.tree)
      setImpactFilter(
        changes.removed.length ? 'removed' : changes.modified.length ? 'modified' : 'added',
      )
      setImpactQuery('')
      setNow(Date.now())
    } catch (error) {
      setImportError(apiError(error, '题包预检失败'))
    } finally {
      operation.current = false
      setBusy('')
    }
  }
  async function apply() {
    if (!review || operation.current || applied || appliedReceipt.current === review.receipt.id)
      return
    const current = latest.current
    const problem = importReviewProblem({
      receipt: review.receipt,
      parametersMatch:
        review.file === current.file && review.timeLimit === current.timeLimit.trim(),
      etag: current.copy.etag,
      mergeId: current.copy.mergeId,
      canEdit: current.canEdit,
      now: Date.now(),
    })
    if (problem) {
      setImportError(problem)
      return
    }
    operation.current = true
    setBusy('apply')
    setImportError('')
    try {
      const next = await api.postApiAuthoringProblemsIdImportsImportIdApply(
        problemId,
        review.receipt.id,
        {
          etag: review.receipt.etag,
        },
      )
      appliedReceipt.current = review.receipt.id
      setReview({ ...review, receipt: { ...review.receipt, applied: true } })
      onSaved(next)
      setApplied(true)
    } catch (error) {
      setImportError(apiError(error, '应用失败，原工作副本已保留'))
    } finally {
      operation.current = false
      setBusy('')
    }
  }
  async function verifyExportSource(expected: string) {
    if (expected.startsWith('revision:')) return true
    const current = await api.getApiAuthoringProblemsIdWorkingCopy(problemId)
    if (packageSourceKey(current.etag) === expected) return true
    onSaved(current)
    setExportError('服务器工作副本已变化，请重新生成题包后下载。')
    return false
  }
  async function download(result: ExportReview) {
    if (operation.current) return
    const isCurrent = () =>
      result.sourceKey === latest.current.sourceKey && result.format === latest.current.format
    if (!isCurrent()) {
      setExportError('材料或格式已变化，请重新生成题包。')
      return
    }
    operation.current = true
    setBusy('download')
    setExportError('')
    try {
      if (!(await verifyExportSource(result.sourceKey))) return
      const blob = await api.getApiAuthoringProblemsIdBlobsDigest(
        problemId,
        result.value.file.sha256,
      )
      if (!isCurrent()) {
        setExportError('下载准备期间材料已变化，请重新生成题包。')
        return
      }
      if (!(await verifyExportSource(result.sourceKey))) return
      const url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = result.value.filename
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      setExportError(apiError(error, '下载失败，可重试下载'))
    } finally {
      operation.current = false
      setBusy('')
    }
  }
  async function exportArchive() {
    if (operation.current || (!canEdit && !revision)) return
    operation.current = true
    setBusy('export')
    setExportError('')
    const exportedSource = {
      sourceKey,
      sourceLabel: revision
        ? sourceLabel
        : `工作副本（保存于 ${new Date(copy.updatedAt).toLocaleString()}）`,
      format,
    }
    try {
      if (!(await verifyExportSource(exportedSource.sourceKey))) return
      const value = await api.postApiAuthoringProblemsIdExports(problemId, { format, revision })
      setExported({ value, ...exportedSource, createdAt: new Date().toLocaleString() })
      await verifyExportSource(exportedSource.sourceKey)
    } catch (error) {
      setExportError(apiError(error, '题包导出失败'))
    } finally {
      operation.current = false
      setBusy('')
    }
  }
  const impactItems = (impact?.[impactFilter] ?? []).filter((change) => {
    const entry = change.after ?? change.before!
    return `${entryLabel(entry)} ${entry.path} ${change.before?.path ?? ''}`
      .toLowerCase()
      .includes(impactQuery.trim().toLowerCase())
  })
  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-xl font-semibold tracking-tight">导入与导出</h2>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">
          从其他平台带入材料，或者把当前内容打包带走。选择你这次要做的操作。
        </p>
      </header>
      {demo && (
        <p className="text-sm leading-6 text-muted-foreground">
          演示环境支持 Vertex 原生归档往返，以及不含 config.yml 的平铺数据 ZIP，最大 8
          MiB。标准格式题包请使用真实后端。
        </p>
      )}
      <div className="flex gap-1 border-b pb-3">
        {canEdit && (
          <Button
            variant={mode === 'import' ? 'secondary' : 'ghost'}
            onClick={() => setMode('import')}
            disabled={!!busy}
          >
            <Upload />
            导入题包
          </Button>
        )}
        <Button
          variant={mode === 'export' ? 'secondary' : 'ghost'}
          onClick={() => setMode('export')}
          disabled={!!busy}
        >
          <Download />
          导出材料
        </Button>
      </div>
      <div className="max-w-5xl">
        {canEdit && mode === 'import' && (
          <section className="rounded-xl border bg-card p-5 space-y-4" aria-label="导入题包">
            <ol className="grid grid-cols-3 gap-2 border-b pb-4" aria-label="导入步骤">
              {['选择题包', '预检与审阅', '应用到副本'].map((label, index) => {
                const current = applied ? 2 : receipt && parametersMatch ? 1 : 0
                return (
                  <li
                    key={label}
                    aria-current={current === index ? 'step' : undefined}
                    className={cn(
                      'flex items-center gap-2 text-xs',
                      current === index ? 'font-semibold text-primary' : 'text-muted-foreground',
                    )}
                  >
                    <span
                      className={cn(
                        'flex size-6 shrink-0 items-center justify-center rounded-full border',
                        index <= current && 'border-primary/30 bg-primary/5',
                      )}
                    >
                      {index < current ? <CheckCircle2 className="size-3.5" /> : index + 1}
                    </span>
                    {label}
                  </li>
                )
              })}
            </ol>
            <div className="flex items-center gap-2">
              <Upload className="size-4 text-muted-foreground" />
              <h3 className="font-medium">导入材料</h3>
            </div>
            <p className="text-sm leading-6 text-muted-foreground">
              支持 Vertex 原生归档、Kattis / ICPC、已生成数据的 Polygon 包和洛谷数据 ZIP。
            </p>
            <input
              ref={upload}
              type="file"
              accept=".zip,.kpp"
              className="hidden"
              disabled={Boolean(busy) || Boolean(copy.mergeId)}
              aria-label="选择题包文件"
              onChange={(event) => {
                const chosen = event.target.files?.[0]
                if (chosen) chooseFile(chosen)
                event.target.value = ''
              }}
            />
            <Button
              variant="outline"
              className="h-auto w-full flex-col border-2 border-dashed bg-muted/20 px-6 py-9"
              disabled={Boolean(busy) || Boolean(copy.mergeId)}
              onClick={() => upload.current?.click()}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault()
                if (operation.current || copy.mergeId) return
                const chosen = event.dataTransfer.files[0]
                if (chosen) {
                  chooseFile(chosen)
                }
              }}
            >
              <FileArchive className="mb-2 size-8 text-primary" />
              <span className="max-w-full truncate">
                {file?.name ?? '拖入 ZIP / KPP，或点击选择'}
              </span>
              <span className="text-xs font-normal text-muted-foreground">
                {file
                  ? `${formatFileSize(file.size)} · 点击更换题包`
                  : '选择文件后先预检，审阅影响再应用'}
              </span>
            </Button>
            <details className="text-sm">
              <summary className="cursor-pointer text-muted-foreground">
                旧格式题包的时间限制
              </summary>
              <div className="mt-3">
                <Field
                  label="固定时限（ms，可选）"
                  hint="legacy 题包可能只包含参考解倍率。填写后采用此固定时限；已有明确时限时以题包为准。"
                >
                  <Input
                    aria-label="导入固定时限"
                    type="number"
                    min={1}
                    max={3600000}
                    value={timeLimit}
                    onChange={(event) => {
                      setTimeLimit(event.target.value)
                      setApplied(false)
                      setImportError('')
                    }}
                    disabled={Boolean(busy)}
                    placeholder="例如 1000"
                  />
                </Field>
              </div>
            </details>
            <div className="flex flex-wrap items-center gap-3">
              <Button
                variant={receipt && !reviewProblem ? 'outline' : 'default'}
                disabled={!file || Boolean(busy) || Boolean(copy.mergeId)}
                loading={busy === 'preview'}
                onClick={() => void preview()}
              >
                {receipt ? <RefreshCw /> : <Archive />}
                {receipt ? '重新预检' : '预检题包'}
              </Button>
              <p className="text-xs text-muted-foreground">预检只分析题包，不会修改工作副本。</p>
            </div>
            {copy.mergeId && (
              <p className="text-sm text-destructive">
                请先在“更改与提交”解决合并冲突，再导入题包。
              </p>
            )}
            {importError && (
              <p
                role="alert"
                className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
              >
                {importError}
              </p>
            )}
            {receipt && (
              <div className="space-y-4 border-t pt-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h3 className="text-base font-semibold">
                      {applied ? '本次导入记录' : '审阅导入影响'}
                    </h3>
                    <p className="mt-1 break-all text-xs text-muted-foreground">
                      {review?.file.name} ·{' '}
                      {formatLabels[receipt.plan.format] || receipt.plan.format} ·{' '}
                      {receipt.plan.fileCount} 份材料
                    </p>
                  </div>
                  {!applied && (
                    <span className="text-xs text-muted-foreground">
                      预检有效至 {new Date(receipt.expiresAt).toLocaleTimeString()}
                    </span>
                  )}
                </div>
                {!applied && reviewProblem && (
                  <p
                    role="status"
                    className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm leading-6 text-amber-800 dark:text-amber-300"
                  >
                    <CircleAlert className="mt-1 size-4 shrink-0" />
                    {reviewProblem}
                  </p>
                )}
                <div
                  className={cn(
                    'rounded-lg border p-3 text-sm leading-6',
                    receipt.plan.scope === 'data'
                      ? 'bg-muted/30'
                      : 'border-amber-500/25 bg-amber-500/5',
                  )}
                >
                  <p className="font-medium">
                    {receipt.plan.scope === 'data' ? '合并测试数据' : '替换整个工作副本'}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {receipt.plan.scope === 'data'
                      ? '数据将按预检计划合并，题面和其他材料保留。请检查同名数据的更新。'
                      : '未包含在题包中的当前材料会从工作副本移除。已有提交历史保留，请重点核对下方移除清单。'}
                  </p>
                </div>
                {impact && (
                  <div className="rounded-lg border">
                    <div
                      className="grid grid-cols-3 gap-2 border-b bg-muted/20 p-2"
                      aria-label="导入影响筛选"
                    >
                      {(['added', 'modified', 'removed'] as const).map((kind) => (
                        <button
                          key={kind}
                          type="button"
                          aria-pressed={impactFilter === kind}
                          onClick={() => {
                            setImpactFilter(kind)
                            setImpactQuery('')
                          }}
                          className={cn(
                            'rounded-lg px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                            impactFilter === kind
                              ? 'bg-card shadow-sm ring-1 ring-border'
                              : 'hover:bg-muted/50',
                          )}
                        >
                          <span
                            className={cn(
                              'block text-lg font-semibold tabular-nums',
                              kind === 'removed' && impact[kind].length > 0 && 'text-destructive',
                            )}
                          >
                            {impact[kind].length}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {{ added: '新增材料', modified: '修改材料', removed: '移除材料' }[kind]}
                          </span>
                        </button>
                      ))}
                    </div>
                    <div className="p-3">
                      <Input
                        aria-label="筛选导入影响材料"
                        placeholder="按材料名称或路径查找…"
                        value={impactQuery}
                        onChange={(event) => setImpactQuery(event.target.value)}
                      />
                    </div>
                    <ul className="max-h-72 divide-y overflow-auto border-t">
                      {impactItems.map((change) => {
                        const entry = change.after ?? change.before!
                        return (
                          <li
                            key={change.entryId}
                            className="flex items-start justify-between gap-3 px-3 py-3"
                          >
                            <div className="min-w-0">
                              <p className="break-words text-sm font-medium">{entryLabel(entry)}</p>
                              <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                                {entry.path}
                              </p>
                              {change.before &&
                                change.after &&
                                change.before.path !== change.after.path && (
                                  <p className="mt-1 break-all text-xs text-muted-foreground">
                                    原路径：{change.before.path}
                                  </p>
                                )}
                            </div>
                            <span className="shrink-0 rounded bg-muted px-2 py-1 text-[11px] text-muted-foreground">
                              {materialNames[entry.kind] || entry.kind}
                            </span>
                          </li>
                        )
                      })}
                      {!impactItems.length && (
                        <li className="px-3 py-6 text-center text-xs text-muted-foreground">
                          {impactQuery ? '没有匹配的材料' : '这一类没有变化'}
                        </li>
                      )}
                    </ul>
                    <p className="border-t px-3 py-2 text-[11px] text-muted-foreground">
                      清单对比预检时的工作副本；未变化的材料不列出。
                    </p>
                  </div>
                )}
                <PackageIssues issues={receipt.plan.issues} />
                {applied ? (
                  <div
                    className="rounded-lg border border-emerald-500/25 bg-emerald-500/5 p-4"
                    role="status"
                  >
                    <p className="flex items-center gap-2 text-sm font-medium">
                      <CheckCircle2 className="size-4 text-emerald-600" />
                      已应用到工作副本
                    </p>
                    <p className="mt-2 text-xs leading-5 text-muted-foreground">
                      继续核对题面和数据，再运行检查。需要与协作者共享时，前往“更改与提交”。
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button size="sm" variant="outline" asChild>
                        <Link
                          to={`/authoring/${problemId}/${receipt.plan.scope === 'data' ? 'tests' : 'statement'}`}
                        >
                          继续编辑
                          <ArrowRight />
                        </Link>
                      </Button>
                      <Button size="sm" asChild>
                        <Link to={`/authoring/${problemId}/checks`}>
                          运行检查
                          <ArrowRight />
                        </Link>
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
                    <p className="text-xs text-muted-foreground">
                      {receipt.plan.scope === 'data'
                        ? '确认以上数据合并计划后应用。'
                        : '确认以上替换与移除清单后应用。'}
                    </p>
                    <Button
                      disabled={Boolean(reviewProblem) || Boolean(busy)}
                      loading={busy === 'apply'}
                      onClick={() => void apply()}
                    >
                      {receipt.plan.scope === 'data' ? '确认合并到工作副本' : '确认替换工作副本'}
                    </Button>
                  </div>
                )}
              </div>
            )}
          </section>
        )}
        {(mode === 'export' || !canEdit) && (
          <section className="rounded-xl border bg-card p-5 space-y-4" aria-label="导出题包">
            <div className="grid gap-2 sm:grid-cols-3">
              {[
                ['vertex', '完整备份', '包含题面、程序和全部材料'],
                ['luogu-data', '仅测试数据', '传递输入、答案与逐点限制'],
                ['standard', '交付其他平台', 'ICPC、Kattis 或 DOMjudge'],
              ].map(([id, label, hint]) => (
                <button
                  key={id}
                  type="button"
                  disabled={!!busy}
                  aria-pressed={
                    id === 'standard' ? !['vertex', 'luogu-data'].includes(format) : format === id
                  }
                  onClick={() => {
                    setFormat(id === 'standard' ? 'kattis-2025-09' : id)
                    setExportError('')
                  }}
                  className={`rounded-xl border p-4 text-left transition-colors ${(id === 'standard' ? !['vertex', 'luogu-data'].includes(format) : format === id) ? 'border-primary bg-primary/5' : 'hover:border-primary/40'}`}
                >
                  <span className="block text-sm font-medium">{label}</span>
                  <span className="mt-2 block text-xs leading-5 text-muted-foreground">{hint}</span>
                </button>
              ))}
            </div>
            <div className="flex items-center gap-2">
              <Archive className="size-4 text-muted-foreground" />
              <h3 className="font-medium">导出材料</h3>
            </div>
            {!['vertex', 'luogu-data'].includes(format) && (
              <Choice
                label="题包格式"
                value={format}
                onChange={(value) => {
                  setFormat(value)
                  setExportError('')
                }}
                disabled={Boolean(busy)}
                options={[
                  ['kattis-legacy-icpc', 'ICPC legacy-icpc'],
                  ['kattis-legacy', 'Kattis legacy'],
                  ['kattis-2025-09', 'ICPC / Kattis 2025-09'],
                  ['domjudge', 'DOMjudge（legacy + 固定时限）'],
                ]}
              />
            )}
            <p className="text-sm leading-6 text-muted-foreground">
              {format === 'vertex'
                ? '完整保留材料、文件布局和未处理配置。适合备份及迁移，导入后仍需显式提交。'
                : format === 'luogu-data'
                  ? '按当前测试顺序导出输入、答案和逐点限制。此格式只传递数据，不包含题面、程序、样例标记和分组计分规则。'
                  : '标准格式需要可执行的校验器和完整数据。生成型测试使用匹配的成功检查；不能映射的规则会阻止导出。'}
            </p>
            <div className="grid gap-3 rounded-lg bg-muted/40 p-4 sm:grid-cols-2">
              <div>
                <p className="text-xs text-muted-foreground">导出来源</p>
                <p className="mt-1 text-sm font-medium">{sourceLabel}</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {revision
                    ? '已提交的固定版本'
                    : `副本保存于 ${new Date(copy.updatedAt).toLocaleString()}`}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">题包格式</p>
                <p className="mt-1 text-sm font-medium">{formatLabels[format] || format}</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  生成后可核对文件与兼容提醒，再下载。
                </p>
              </div>
            </div>
            {exportError && (
              <p
                role="alert"
                className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
              >
                {exportError}
              </p>
            )}
            <Button
              variant="outline"
              loading={busy === 'export'}
              disabled={Boolean(busy) || (!canEdit && !revision)}
              onClick={() => void exportArchive()}
            >
              {exported ? <RefreshCw /> : <Archive />}
              {exported ? '重新生成题包' : '生成题包'}
            </Button>
            {exported && (
              <div className="rounded-lg border p-4 space-y-3 text-sm" aria-live="polite">
                {exportStale && (
                  <p className="flex items-start gap-2 rounded-lg bg-amber-500/10 p-3 text-xs leading-5 text-amber-800 dark:text-amber-300">
                    <CircleAlert className="mt-0.5 size-4 shrink-0" />
                    材料来源或格式已变化，下面是先前生成的题包。请重新生成后下载。
                  </p>
                )}
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="break-all font-medium">{exported.value.filename}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {formatFileSize(exported.value.file.bytes)} ·{' '}
                      {formatLabels[exported.value.format] || exported.value.format}
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={exportStale || Boolean(busy)}
                    loading={busy === 'download'}
                    onClick={() => void download(exported)}
                  >
                    <Download />
                    下载题包
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  生成来源：{exported.sourceLabel} · {exported.createdAt}
                </p>
                <PackageIssues issues={exported.value.issues} />
              </div>
            )}
          </section>
        )}
      </div>
    </div>
  )
}
