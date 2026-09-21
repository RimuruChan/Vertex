import { materialChanges, materialValue } from './material-diff'
import { Link } from '@/domain/navigation'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Download } from 'lucide-react'
import type {
  DomainContentComparison,
  DomainContentCommit,
  DomainContentChange,
  DomainWorkingCopy,
  DomainTreeEntry,
  DomainMergeSession,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/input'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError, formatDateTime } from '@/lib/format'
import { entryLabel, materialNames, sameValue, isDocument } from '@/lib/authoring-materials'
import { lineDiff } from '@/lib/line-diff'
import { cn } from '@/lib/utils'
import ConflictEditor from './ConflictEditor'

export function DiffContent({
  problemId,
  change,
  labels = {},
  beforeLabel = '修改前',
  afterLabel = '修改后',
}: {
  problemId: string
  change: DomainContentChange
  labels?: Record<string, string>
  beforeLabel?: string
  afterLabel?: string
}) {
  const api = useDomainAPI(),
    [values, setValues] = useState<string[]>([]),
    [error, setError] = useState('')
  const [raw, setRaw] = useState(false),
    [nonText, setNonText] = useState(false)
  const [downloadError, setDownloadError] = useState(''),
    [downloading, setDownloading] = useState(false)
  const binary =
    nonText ||
    [change.before, change.after].some(
      (entry) =>
        entry &&
        (entry.blob.bytes > 1 << 20 ||
          ['asset', 'resource'].includes(entry.kind) ||
          (['input', 'answer'].includes(entry.kind) && entry.blob.bytes > 65536) ||
          entry.attributes.format === 'pdf'),
    )
  const diff = useMemo(
    () => (!binary && values.length === 2 ? lineDiff(values[0], values[1]) : undefined),
    [values, binary],
  )
  async function download(entry: DomainTreeEntry) {
    setDownloading(true)
    setDownloadError('')
    try {
      const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256),
        url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = `${entry === change.before ? 'before' : 'after'}-${entry.path.split('/').pop() || 'material'}`
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      setDownloadError(apiError(error, '文件下载失败'))
    } finally {
      setDownloading(false)
    }
  }
  useEffect(() => {
    let active = true
    setValues([])
    setRaw(false)
    setNonText(false)
    setError('')
    setDownloadError('')
    setError('')
    Promise.all(
      [change.before, change.after].map(async (entry) => {
        if (!entry) return ''
        if (
          entry.blob.bytes > 1 << 20 ||
          ['asset', 'resource'].includes(entry.kind) ||
          (['input', 'answer'].includes(entry.kind) && entry.blob.bytes > 65536) ||
          entry.attributes.format === 'pdf'
        )
          return `${entryLabel(entry)}\n${entry.blob.bytes.toLocaleString()} 字节`
        const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256)
        let text: string
        try {
          text = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
          if (text.includes('\0')) throw new Error('binary')
        } catch {
          if (active) setNonText(true)
          return `${entryLabel(entry)} · ${entry.blob.bytes} 字节`
        }
        if (!isDocument(entry.kind)) return text
        try {
          return JSON.stringify(JSON.parse(text), null, 2)
        } catch {
          return text
        }
      }),
    )
      .then((value) => {
        if (active) setValues(value)
      })
      .catch((error) => {
        if (active) setError(apiError(error, '读取差异失败'))
      })
    return () => {
      active = false
    }
  }, [api, problemId, change])
  const structured =
    !binary && values.length === 2 && isDocument((change.after ?? change.before)!.kind)
      ? materialChanges(values[0], values[1])
      : null
  if (error)
    return (
      <p role="alert" className="text-sm text-destructive">
        {error}
      </p>
    )
  return (
    <div className="min-w-0 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">
            {labels[change.entryId] ?? entryLabel((change.after ?? change.before)!)}
          </p>
        </div>
        <div className="flex gap-2">
          {structured && (
            <Button size="sm" variant="ghost" onClick={() => setRaw((value) => !value)}>
              {raw ? '字段变化' : '原始文件'}
            </Button>
          )}
          {(
            [
              [beforeLabel, change.before],
              [afterLabel, change.after],
            ] as const
          ).map(
            ([label, entry]) =>
              entry && (
                <Button
                  key={label}
                  variant="outline"
                  size="sm"
                  disabled={downloading}
                  onClick={() => void download(entry)}
                >
                  <Download />
                  下载{label}
                </Button>
              ),
          )}
        </div>
      </div>
      {downloadError && (
        <p role="alert" className="text-sm text-destructive">
          {downloadError}
        </p>
      )}
      {(change.before?.kind !== change.after?.kind ||
        !sameValue(change.before?.attributes, change.after?.attributes)) && (
        <details className="rounded-lg border p-3 text-xs">
          <summary className="cursor-pointer font-medium">材料属性变化</summary>
          <div className="mt-3 grid min-w-0 gap-3 sm:grid-cols-2">
            {[change.before, change.after].map((entry, index) => (
              <pre key={index} className="overflow-auto whitespace-pre-wrap break-words leading-5">
                {index === 0 ? beforeLabel : afterLabel}
                {'\n'}
                {entry
                  ? `${materialNames[entry.kind] || entry.kind}\n${JSON.stringify(entry.attributes, null, 2)}`
                  : '（不存在）'}
              </pre>
            ))}
          </div>
        </details>
      )}
      {structured && !raw ? (
        <div className="overflow-hidden rounded-xl border">
          <div className="grid grid-cols-[minmax(100px,1fr)_minmax(0,2fr)_minmax(0,2fr)] gap-3 border-b bg-muted/30 px-4 py-3 text-xs text-muted-foreground">
            <span>设置</span>
            <span>{beforeLabel}</span>
            <span>{afterLabel}</span>
          </div>
          {structured.map((field) => (
            <div
              key={field.key}
              className="grid grid-cols-[minmax(100px,1fr)_minmax(0,2fr)_minmax(0,2fr)] gap-3 border-b px-4 py-3 text-sm last:border-0"
            >
              <span className="font-medium">{field.label}</span>
              <span className="break-words text-muted-foreground">
                {materialValue(field.key, field.before, labels)}
              </span>
              <span className="break-words text-primary">
                {materialValue(field.key, field.after, labels)}
              </span>
            </div>
          ))}
          {!structured.length && (
            <p className="p-5 text-sm text-muted-foreground">内容一致，仅文件属性发生变化。</p>
          )}
        </div>
      ) : binary ? (
        <div className="space-y-3 rounded-xl border p-4">
          <p className="text-sm text-muted-foreground">
            数据、二进制或较大文件不内联比较，可下载完整内容核对。
          </p>
          <div className="grid min-w-0 gap-3 sm:grid-cols-2">
            {[beforeLabel, afterLabel].map((label, index) => (
              <div key={label}>
                <h4 className="text-xs font-medium">{label}</h4>
                <pre className="mt-2 whitespace-pre-wrap break-words text-xs text-muted-foreground">
                  {values[index] || '（不存在）'}
                </pre>
              </div>
            ))}
          </div>
        </div>
      ) : !diff ? (
        <p className="py-8 text-sm text-muted-foreground">正在读取差异…</p>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <div className="flex items-center gap-4 border-b bg-muted/30 px-3 py-2 text-xs">
            <span className="font-medium">
              {beforeLabel} → {afterLabel}
            </span>
            <span className="text-emerald-700 dark:text-emerald-400">
              +{diff.lines.filter((line) => line.kind === 'added').length} 新增
            </span>
            <span className="text-rose-700 dark:text-rose-400">
              −{diff.lines.filter((line) => line.kind === 'removed').length} 删除
            </span>
          </div>
          {(diff.truncated || diff.coarse) && (
            <p className="border-b px-3 py-2 text-xs text-muted-foreground">
              {diff.truncated
                ? '文件较长，仅预览前面的内容；可下载完整文件核对。'
                : '差异较大，按整段替换显示。'}
            </p>
          )}
          <div className="max-h-[480px] overflow-auto font-mono text-xs leading-6">
            <table className="w-full table-fixed">
              <colgroup>
                <col className="w-10" />
                <col className="w-10" />
                <col className="w-5" />
                <col />
              </colgroup>
              <thead className="sr-only">
                <tr>
                  <th>{beforeLabel}行号</th>
                  <th>{afterLabel}行号</th>
                  <th>操作</th>
                  <th>内容</th>
                </tr>
              </thead>
              <tbody>
                {diff.lines.map((line, index) => (
                  <tr
                    key={index}
                    className={cn(
                      line.kind === 'added' && 'bg-emerald-500/10',
                      line.kind === 'removed' && 'bg-rose-500/10',
                    )}
                  >
                    <td className="select-none align-top pr-2 text-right text-muted-foreground/70">
                      {line.before}
                    </td>
                    <td className="select-none align-top pr-2 text-right text-muted-foreground/70">
                      {line.after}
                    </td>
                    <td
                      className={cn(
                        'select-none align-top',
                        line.kind === 'added'
                          ? 'text-emerald-700 dark:text-emerald-400'
                          : 'text-rose-700 dark:text-rose-400',
                      )}
                    >
                      {line.kind === 'added' ? '+' : line.kind === 'removed' ? '−' : ''}
                    </td>
                    <td className="pr-3 align-top">
                      <pre className="whitespace-pre-wrap break-words font-mono">
                        {line.text || '\u00a0'}
                      </pre>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function ConflictPanel({
  problemId,
  copy,
  onSaved,
  onDirty,
  onBusy,
}: {
  problemId: string
  copy: DomainWorkingCopy
  onSaved: (copy: DomainWorkingCopy) => void
  onDirty: (dirty: boolean) => void
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI(),
    [session, setSession] = useState<DomainMergeSession>(),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const dirtyEntries = useRef(new Set<string>())
  const markDirty = useCallback(
    (key: string, dirty: boolean) => {
      if (dirty) dirtyEntries.current.add(key)
      else dirtyEntries.current.delete(key)
      onDirty(dirtyEntries.current.size > 0)
    },
    [onDirty],
  )
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    let active = true
    api
      .getApiAuthoringProblemsIdMergesMergeId(problemId, copy.mergeId!)
      .then((value) => {
        if (active) setSession(value)
      })
      .catch((error) => {
        if (active) setError(apiError(error, '读取冲突失败'))
      })
    return () => {
      active = false
    }
  }, [api, problemId, copy.mergeId])
  async function choose(index: number, side: 'local' | 'remote' | 'base') {
    if (!session) return
    setBusy(true)
    setError('')
    try {
      const conflict = session.result.conflicts[index],
        selected = conflict[side],
        tree = structuredClone(session.result.tree),
        position = tree.entries.findIndex((e) => e.id === conflict.entryId)
      if (conflict.field === 'entry') {
        tree.entries = tree.entries.filter((e) => e.id !== conflict.entryId)
        if (selected) tree.entries.push(selected)
      } else if (position >= 0) {
        const entry = tree.entries[position]
        if (conflict.field === 'blob' && selected) entry.blob = selected.blob
        else if (conflict.field === 'path' && selected) entry.path = selected.path
        else if (conflict.field === 'kind' && selected) entry.kind = selected.kind
        else if (conflict.field.startsWith('attributes.')) {
          const key = conflict.field.slice(11)
          if (selected && key in selected.attributes)
            entry.attributes[key] = selected.attributes[key]
          else delete entry.attributes[key]
        }
      }
      const value = await api.putApiAuthoringProblemsIdMergesMergeId(problemId, session.id, {
        etag: session.etag,
        tree,
        resolved: [{ entryId: conflict.entryId, field: conflict.field }],
      })
      setSession(value)
    } catch (error) {
      setError(apiError(error, '保存冲突处理失败'))
    } finally {
      setBusy(false)
    }
  }
  async function complete() {
    if (!session) return
    setBusy(true)
    try {
      onSaved(
        await api.postApiAuthoringProblemsIdMergesMergeIdComplete(problemId, session.id, {
          etag: session.etag,
        }),
      )
    } catch (error) {
      setError(apiError(error, '完成合并失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-5">
      <div>
        <h2 className="text-lg font-semibold">解决协作冲突</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          双方修改均已保留。逐项确认后完成合并，再提交更改。
        </p>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {session?.result.conflicts.map((conflict, index) => (
        <section
          key={`${conflict.entryId}:${conflict.field}`}
          aria-label={`解决 ${(conflict.local ?? conflict.remote)?.path} 的冲突`}
          className="space-y-3 rounded-xl border p-4"
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium">{(conflict.local ?? conflict.remote)?.path}</h3>
              <p className="mt-1 text-xs text-muted-foreground">
                {conflict.kind === 'delete-modify'
                  ? '一方删除，另一方修改'
                  : conflict.kind === 'path-collision'
                    ? '文件路径冲突'
                    : '双方修改了相同内容'}{' '}
                ·{' '}
                {conflict.field === 'blob'
                  ? '内容'
                  : conflict.field === 'path'
                    ? '文件路径'
                    : conflict.field === 'kind'
                      ? '材料类型'
                      : conflict.field === 'entry'
                        ? '整个材料'
                        : '材料属性'}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" disabled={busy} onClick={() => void choose(index, 'local')}>
                保留我的
              </Button>
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => void choose(index, 'remote')}
              >
                采用最新
              </Button>
              <Button variant="ghost" disabled={busy} onClick={() => void choose(index, 'base')}>
                恢复基线
              </Button>
            </div>
          </div>
          <DiffContent
            problemId={problemId}
            beforeLabel="我的副本"
            afterLabel="共享最新"
            change={{
              entryId: conflict.entryId,
              kind: 'modified',
              before: conflict.local,
              after: conflict.remote,
            }}
          />
          <ConflictEditor
            problemId={problemId}
            entries={session.result.tree.entries}
            conflict={conflict}
            entry={session.result.tree.entries.find((entry) => entry.id === conflict.entryId)}
            disabled={busy}
            onDirty={markDirty}
            onResolve={async (entry) => {
              setBusy(true)
              try {
                const tree = structuredClone(session.result.tree)
                const position = tree.entries.findIndex((item) => item.id === conflict.entryId)
                if (position < 0) tree.entries.push(entry)
                else tree.entries[position] = entry
                const value = await api.putApiAuthoringProblemsIdMergesMergeId(
                  problemId,
                  session.id,
                  {
                    etag: session.etag,
                    tree,
                    resolved: [{ entryId: conflict.entryId, field: conflict.field }],
                  },
                )
                setSession(value)
              } finally {
                setBusy(false)
              }
            }}
          />
        </section>
      ))}
      {session?.result.conflicts.length === 0 && (
        <div className="surface-panel space-y-3 p-5">
          <p className="text-sm">冲突已处理，可以将结果保存回工作副本。</p>
          <Button loading={busy} onClick={() => void complete()}>
            完成合并
          </Button>
        </div>
      )}
    </div>
  )
}

export default function ChangesPanel({
  problemId,
  copy,
  canEdit,
  history = false,
  onSaved,
  onDirty,
  onBusy,
}: {
  problemId: string
  copy: DomainWorkingCopy
  canEdit: boolean
  history?: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onDirty: (dirty: boolean) => void
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI(),
    confirm = useConfirm(),
    [comparison, setComparison] = useState<DomainContentComparison>(),
    [commits, setCommits] = useState<DomainContentCommit[]>([]),
    [revision, setRevision] = useState<number>(),
    [selected, setSelected] = useState(''),
    [message, setMessage] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false),
    [reload, setReload] = useState(0)
  const [committed, setCommitted] = useState<number>()
  const [conflictDirty, setConflictDirty] = useState(false),
    [conflictBusy, setConflictBusy] = useState(false)
  const [hasMore, setHasMore] = useState(false),
    [loadingMore, setLoadingMore] = useState(false),
    historyGeneration = useRef(0)
  useEffect(() => {
    onDirty(Boolean(message) || conflictDirty)
    return () => onDirty(false)
  }, [message, conflictDirty, onDirty])
  useEffect(() => {
    onBusy(busy || conflictBusy)
    return () => onBusy(false)
  }, [busy, conflictBusy, onBusy])
  const attempt = useRef<{ etag: string; message: string; requestId: string } | undefined>(
    undefined,
  )
  useEffect(() => {
    let active = true
    historyGeneration.current++
    setError('')
    api
      .getApiAuthoringProblemsIdCommits(problemId, { limit: 30 })
      .then((value) => {
        if (active) {
          setCommits(value.items)
          setHasMore(value.items.length === 30)
        }
      })
      .catch((error) => {
        if (active) setError(apiError(error, '历史读取失败'))
      })
    return () => {
      active = false
    }
  }, [api, problemId, copy.etag, reload])
  async function moreHistory() {
    if (loadingMore || !commits.length) return
    const generation = historyGeneration.current
    setLoadingMore(true)
    try {
      const value = await api.getApiAuthoringProblemsIdCommits(problemId, {
        limit: 30,
        before: commits.at(-1)!.revision,
      })
      if (generation !== historyGeneration.current) return
      setCommits((current) => [
        ...current,
        ...value.items.filter(
          (item) => !current.some((existing) => existing.revision === item.revision),
        ),
      ])
      setHasMore(value.items.length === 30)
    } catch (error) {
      if (generation === historyGeneration.current) setError(apiError(error, '加载更多历史失败'))
    } finally {
      setLoadingMore(false)
    }
  }
  const [labels, setLabels] = useState<Record<string, string>>({ problem: '题目设置' })
  useEffect(() => {
    let live = true
    const initial = Object.fromEntries(
      copy.tree.entries.map((entry) => [
        entry.id,
        entry.id === 'problem' ? '题目设置' : entryLabel(entry),
      ]),
    )
    setLabels(initial)
    const selectedRevision = history || !canEdit ? (revision ?? commits[0]?.revision) : undefined
    if ((history || !canEdit) && !selectedRevision) return
    void Promise.all(
      ['program', 'test', 'group', 'validation'].map((kind) =>
        api.getApiAuthoringProblemsIdMaterials(problemId, {
          kind,
          limit: 100,
          revision: selectedRevision,
        }),
      ),
    )
      .then((pages) => {
        if (live)
          setLabels({
            ...initial,
            ...Object.fromEntries(
              pages.flatMap((page) =>
                page.items.map((item) => [
                  item.entry.id,
                  item.program?.name ??
                    item.test?.name ??
                    item.group?.name ??
                    item.validation?.name ??
                    entryLabel(item.entry),
                ]),
              ),
            ),
          })
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [api, problemId, copy.etag, history, canEdit, revision, commits[0]?.revision])
  const target = history || !canEdit ? (revision ?? commits[0]?.revision) : undefined
  useEffect(() => {
    if (copy.mergeId && !history) return
    if ((history || !canEdit) && !target) {
      setComparison(undefined)
      return
    }
    let active = true
    api
      .getApiAuthoringProblemsIdChanges(problemId, target ? { revision: target } : undefined)
      .then((value) => {
        if (active) {
          setComparison(value)
          setSelected((current) =>
            value.changes.some((e) => e.entryId === current)
              ? current
              : value.changes[0]?.entryId || '',
          )
        }
      })
      .catch((error) => {
        if (active) setError(apiError(error, '读取差异失败'))
      })
    return () => {
      active = false
    }
  }, [api, problemId, copy.etag, copy.mergeId, target, history, canEdit, reload])
  async function commit() {
    if (busy) return
    if (!message.trim()) {
      setError('请填写这次更改的说明。')
      return
    }
    setBusy(true)
    setError('')
    try {
      if (attempt.current?.etag !== copy.etag || attempt.current?.message !== message.trim())
        attempt.current = {
          etag: copy.etag,
          requestId: crypto.randomUUID(),
          message: message.trim(),
        }
      const result = await api.postApiAuthoringProblemsIdCommits(problemId, attempt.current)
      setMessage('')
      setCommitted(result.commit?.revision)
      onSaved(result.copy)
      setReload((v) => v + 1)
    } catch (error) {
      setError(apiError(error, '提交失败，工作副本已保留'))
    } finally {
      setBusy(false)
    }
  }
  async function restore() {
    if (
      !target ||
      !(await confirm({
        title: `恢复 r${target} 到工作副本？`,
        description: '当前未提交的修改会被替换，提交历史和已发布版本保持不变。',
        confirmLabel: '恢复到副本',
        destructive: true,
      }))
    )
      return
    setBusy(true)
    try {
      onSaved(
        await api.postApiAuthoringProblemsIdWorkingCopyRestore(problemId, {
          etag: copy.etag,
          revision: target,
        }),
      )
    } catch (error) {
      setError(apiError(error, '恢复失败'))
    } finally {
      setBusy(false)
    }
  }
  if (copy.mergeId && !history)
    return (
      <ConflictPanel
        problemId={problemId}
        copy={copy}
        onSaved={onSaved}
        onDirty={setConflictDirty}
        onBusy={setConflictBusy}
      />
    )
  const change = comparison?.changes.find((item) => item.entryId === selected)
  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">
            {history ? '提交历史' : '审阅更改'}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {history
              ? '选择一次提交，查看它修改了什么。'
              : '核对本次修改并留下说明，协作者就可以同步这些内容。'}
          </p>
        </div>
      </header>
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/25 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {committed && !history && (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 p-4"
        >
          <p className="text-sm">已记录 r{committed}，可以继续检查或编辑。</p>
          <Button size="sm" variant="outline" asChild>
            <Link to={`/authoring/${problemId}/checks`}>去运行检查</Link>
          </Button>
        </div>
      )}
      {!history && canEdit && (
        <form
          noValidate
          className="rounded-xl border bg-card p-4 sm:p-5"
          onSubmit={(event) => {
            event.preventDefault()
            void commit()
          }}
        >
          <label htmlFor="commit-message" className="text-sm font-medium">
            这次修改了什么？
          </label>
          <div className="mt-3 flex flex-col items-stretch gap-3 sm:flex-row sm:items-end">
            <Textarea
              id="commit-message"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              placeholder="例如：补齐边界测试，修正数据范围"
              maxLength={2000}
              className="min-h-20 flex-1 resize-none"
            />
            <Button
              type="submit"
              className="shrink-0"
              loading={busy}
              disabled={!comparison?.changes.length || !message.trim()}
            >
              提交 {comparison?.changes.length ?? '…'} 项更改
            </Button>
          </div>
          <p className="mt-3 text-xs text-muted-foreground">
            提交记录保留历史。确认对外可用时，再到“检查与发布”创建发布版本。
          </p>
        </form>
      )}
      <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[230px_minmax(0,1fr)]">
        <aside className="min-w-0 overflow-hidden rounded-xl border bg-card xl:sticky xl:top-24">
          <h3 className="border-b px-4 py-3 text-xs font-medium text-muted-foreground">
            {history ? '版本记录' : `修改的文件 · ${comparison?.changes.length ?? 0}`}
          </h3>
          <div className="max-h-[60dvh] overflow-auto">
            {history
              ? commits.map((item) => (
                  <button
                    key={item.revision}
                    onClick={() => setRevision(item.revision)}
                    aria-pressed={target === item.revision}
                    className={`block w-full border-b p-4 text-left last:border-0 hover:bg-muted/30 ${target === item.revision ? 'bg-primary/5' : ''}`}
                  >
                    <span className="text-xs font-medium text-primary">r{item.revision}</span>
                    <span className="mt-1 block break-words text-sm font-medium">
                      {item.message}
                    </span>
                    <span className="mt-2 block text-xs text-muted-foreground">
                      {formatDateTime(item.createdAt)}
                    </span>
                  </button>
                ))
              : comparison?.changes.map((item) => (
                  <button
                    key={item.entryId}
                    onClick={() => setSelected(item.entryId)}
                    className={`flex w-full items-start gap-3 border-b px-4 py-3 text-left text-sm last:border-0 hover:bg-muted/30 ${selected === item.entryId ? 'bg-primary/5 text-primary' : ''}`}
                  >
                    <span className="mt-0.5 w-3 shrink-0 font-mono">
                      {item.kind === 'added' ? '+' : item.kind === 'deleted' ? '−' : '~'}
                    </span>
                    <span className="min-w-0 break-words">
                      {labels[item.entryId] ??
                        entryLabel((item.after ?? item.before) as DomainTreeEntry)}
                    </span>
                  </button>
                ))}
          </div>
          {history && hasMore && (
            <div className="border-t p-3">
              <Button
                size="sm"
                variant="ghost"
                className="w-full"
                loading={loadingMore}
                onClick={() => void moreHistory()}
              >
                更早的提交
              </Button>
            </div>
          )}
          {(history ? !commits.length : comparison?.changes.length === 0) && (
            <p className="p-5 text-xs leading-5 text-muted-foreground">
              {history ? '提交后会在这里留下记录。' : '当前草稿与上次提交一致。'}
            </p>
          )}
        </aside>
        <div className="min-w-0 space-y-4">
          {history && target && (
            <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-4">
              <div>
                <p className="text-sm font-medium">
                  r{target} · {commits.find((item) => item.revision === target)?.message}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">相对于前一版本的变化</p>
              </div>
              {canEdit && (
                <Button variant="outline" size="sm" loading={busy} onClick={() => void restore()}>
                  恢复此版本到草稿
                </Button>
              )}
            </div>
          )}
          {history && !!comparison?.changes.length && (
            <div className="flex flex-wrap gap-2">
              {comparison.changes.map((item) => (
                <Button
                  key={item.entryId}
                  size="sm"
                  variant={selected === item.entryId ? 'secondary' : 'outline'}
                  onClick={() => setSelected(item.entryId)}
                >
                  {labels[item.entryId] ??
                    entryLabel((item.after ?? item.before) as DomainTreeEntry)}
                </Button>
              ))}
            </div>
          )}
          {change ? (
            <DiffContent problemId={problemId} change={change} labels={labels} />
          ) : (
            <div className="rounded-xl border border-dashed px-6 py-20 text-center">
              <p className="text-sm font-medium">
                {!comparison
                  ? '正在读取修改内容…'
                  : history
                    ? '选择记录查看修改内容'
                    : '没有待提交的更改'}
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                {history
                  ? '历史内容可以比较和恢复。'
                  : '继续编辑题面、程序或测试数据后，再回来审阅。'}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
