import { useEffect, useRef, useState } from 'react'
import { Archive, Download, FileArchive, Upload } from 'lucide-react'
import type {
  DomainImportReceipt,
  DomainPackageExport,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Choice, Field } from './MaterialForm'
import { apiError } from '@/lib/format'
import { sameValue } from '@/lib/authoring-materials'

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
  const [file, setFile] = useState<File>(),
    [timeLimit, setTimeLimit] = useState('')
  const [receipt, setReceipt] = useState<DomainImportReceipt>(),
    [exported, setExported] = useState<DomainPackageExport>()
  const [format, setFormat] = useState('vertex'),
    [busy, setBusy] = useState(''),
    [importError, setImportError] = useState(''),
    [exportError, setExportError] = useState(''),
    [applied, setApplied] = useState(false)
  const upload = useRef<HTMLInputElement>(null)
  const demo = import.meta.env.VITE_MOCK === 'true'
  useEffect(() => {
    onBusy(Boolean(busy))
    return () => onBusy(false)
  }, [busy, onBusy])
  async function preview() {
    if (!file) return
    setImportError('')
    setReceipt(undefined)
    setApplied(false)
    const limit = timeLimit.trim() ? Number(timeLimit) : undefined
    if (limit !== undefined && (!Number.isInteger(limit) || limit < 1 || limit > 3600000)) {
      setImportError('时间限制应为 1–3600000 的整数毫秒。')
      return
    }
    if (file.size > (demo ? 8 : 64) * 1024 * 1024) {
      setImportError(
        demo ? '演示题包最大 8 MiB；大题包请使用真实后端。' : '题包超过 64 MiB，请缩小归档后重试。',
      )
      return
    }
    setBusy('preview')
    try {
      setReceipt(
        await api.postApiAuthoringProblemsIdImports(problemId, {
          file,
          etag: copy.etag,
          timeLimitMs: limit,
        }),
      )
    } catch (error) {
      setImportError(apiError(error, '题包预检失败'))
    } finally {
      setBusy('')
    }
  }
  async function apply() {
    if (!receipt) return
    setBusy('apply')
    setImportError('')
    try {
      const next = await api.postApiAuthoringProblemsIdImportsImportIdApply(problemId, receipt.id, {
        etag: receipt.etag,
      })
      onSaved(next)
      setApplied(true)
    } catch (error) {
      setImportError(apiError(error, '应用失败，原工作副本已保留'))
    } finally {
      setBusy('')
    }
  }
  async function download(value: DomainPackageExport) {
    const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, value.file.sha256)
    const url = URL.createObjectURL(blob),
      link = document.createElement('a')
    link.href = url
    link.download = value.filename
    link.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  async function exportArchive() {
    setBusy('export')
    setExportError('')
    setExported(undefined)
    try {
      const value = await api.postApiAuthoringProblemsIdExports(problemId, { format, revision })
      setExported(value)
      await download(value)
    } catch (error) {
      setExportError(apiError(error, '题包导出失败'))
    } finally {
      setBusy('')
    }
  }
  const before = new Map(copy.tree.entries.map((entry) => [entry.id, entry]))
  const after = new Set(receipt?.plan.tree.entries.map((entry) => entry.id))
  const added = receipt?.plan.tree.entries.filter((entry) => !before.has(entry.id)).length ?? 0
  const changed =
    receipt?.plan.tree.entries.filter((entry) => {
      const old = before.get(entry.id)
      return old && !sameValue(old, entry)
    }).length ?? 0
  const removed = receipt ? copy.tree.entries.filter((entry) => !after.has(entry.id)).length : 0
  return (
    <div className="max-w-5xl space-y-6">
      <header>
        <h2 className="text-lg font-semibold">题包</h2>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">
          导入先预检，再应用到自己的工作副本。导出固定当前保存的材料，不会创建提交或发布版本。
        </p>
      </header>
      {demo && (
        <p className="text-sm leading-6 text-muted-foreground">
          演示环境支持 Vertex 原生归档往返，以及不含 config.yml 的平铺数据 ZIP，最大 8
          MiB。标准格式题包请使用真实后端。
        </p>
      )}
      <div className="grid items-start gap-5 xl:grid-cols-2">
        {canEdit && (
          <section className="rounded-xl border bg-card p-5 space-y-4" aria-label="导入题包">
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
              aria-label="选择题包文件"
              onChange={(event) => {
                setFile(event.target.files?.[0])
                setReceipt(undefined)
                setApplied(false)
                setImportError('')
              }}
            />
            <Button
              variant="outline"
              className="w-full justify-start"
              disabled={Boolean(busy)}
              onClick={() => upload.current?.click()}
            >
              <FileArchive />
              <span className="truncate">{file?.name ?? '选择 ZIP / KPP 题包'}</span>
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
                      setReceipt(undefined)
                    }}
                    disabled={Boolean(busy)}
                    placeholder="例如 1000"
                  />
                </Field>
              </div>
            </details>
            <Button
              variant="outline"
              disabled={!file || Boolean(busy) || Boolean(copy.mergeId)}
              loading={busy === 'preview'}
              onClick={() => void preview()}
            >
              预检题包
            </Button>
            {importError && (
              <p
                role="alert"
                className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
              >
                {importError}
              </p>
            )}
            {receipt && (
              <div className="space-y-3 border-t pt-4" aria-live="polite">
                <p className="text-sm font-medium">
                  {receipt.plan.format} · {receipt.plan.fileCount} 份材料
                </p>
                <p className="text-sm text-muted-foreground">
                  {receipt.plan.scope === 'data'
                    ? '合并数据，保留其他材料。'
                    : '用此题包替换工作副本。已有提交历史不变。'}
                </p>
                {!applied && (
                  <p className="text-xs tabular-nums text-muted-foreground">
                    新增 {added} · 更新 {changed} · 移除 {removed}
                  </p>
                )}
                {receipt.plan.issues.length > 0 && (
                  <ul className="max-h-72 space-y-2 overflow-auto text-sm">
                    {receipt.plan.issues.map((issue, index) => (
                      <li key={index} className="rounded-lg border p-3">
                        <p
                          className={
                            issue.severity === 'error' || issue.severity === 'blocking'
                              ? 'text-amber-700 dark:text-amber-400'
                              : ''
                          }
                        >
                          {issue.message}
                        </p>
                        {issue.path && (
                          <p className="mt-1 break-all text-xs text-muted-foreground">
                            {issue.path}
                          </p>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
                {!applied && receipt.etag !== copy.etag && (
                  <p className="text-sm text-destructive">工作副本已变化，请重新预检。</p>
                )}
                <Button
                  disabled={
                    applied || !receipt.plan.canApply || receipt.etag !== copy.etag || Boolean(busy)
                  }
                  loading={busy === 'apply'}
                  onClick={() => void apply()}
                >
                  {applied ? '已应用到工作副本' : '应用到工作副本'}
                </Button>
                {applied && (
                  <p className="text-xs text-muted-foreground">
                    可继续编辑；在“更改与提交”确认后再共享。
                  </p>
                )}
              </div>
            )}
          </section>
        )}
        <section className="rounded-xl border bg-card p-5 space-y-4" aria-label="导出题包">
          <div className="flex items-center gap-2">
            <Archive className="size-4 text-muted-foreground" />
            <h3 className="font-medium">导出材料</h3>
          </div>
          <Choice
            label="题包格式"
            value={format}
            onChange={(value) => {
              setFormat(value)
              setExportError('')
              setExported(undefined)
            }}
            disabled={Boolean(busy)}
            options={[
              ['vertex', 'Vertex 原生归档'],
              ['luogu-data', '洛谷 / 平铺测试数据 ZIP'],
              ['kattis-legacy-icpc', 'ICPC legacy-icpc'],
              ['kattis-legacy', 'Kattis legacy'],
              ['kattis-2025-09', 'ICPC / Kattis 2025-09'],
              ['domjudge', 'DOMjudge（legacy + 固定时限）'],
            ]}
          />
          <p className="text-sm leading-6 text-muted-foreground">
            {format === 'vertex'
              ? '完整保留材料、文件布局和未处理配置。适合备份及迁移，导入后仍需显式提交。'
              : format === 'luogu-data'
                ? '按当前测试顺序导出输入、答案和逐点限制。此格式只传递数据，不包含题面、程序、样例标记和分组计分规则。'
                : '标准格式需要可执行的校验器和完整数据。生成型测试使用匹配的成功检查；不能映射的规则会阻止导出。'}
          </p>
          <p className="text-xs text-muted-foreground">
            来源：{revision ? `提交 r${revision}` : '当前已保存的工作副本'}
          </p>
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
            disabled={Boolean(busy)}
            onClick={() => void exportArchive()}
          >
            <Download />
            导出题包
          </Button>
          {exported && (
            <div className="border-t pt-4 space-y-2 text-sm" aria-live="polite">
              <p>已生成 {exported.filename}</p>
              {exported.issues.map((issue, index) => (
                <p key={index} className="text-muted-foreground">
                  {issue.message}
                </p>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
