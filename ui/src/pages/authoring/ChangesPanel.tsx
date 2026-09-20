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
import { entryLabel, materialNames, sameValue } from '@/lib/authoring-materials'
import { lineDiff } from '@/lib/line-diff'
import { cn } from '@/lib/utils'
import ConflictEditor from './ConflictEditor'

export function DiffContent({
  problemId,
  change,
  beforeLabel = '修改前',
  afterLabel = '修改后',
}: {
  problemId: string
  change: DomainContentChange
  beforeLabel?: string
  afterLabel?: string
}) {
  const api = useDomainAPI(),
    [values, setValues] = useState<string[]>([]),
    [error, setError] = useState('')
  const [downloadError, setDownloadError] = useState(''),
    [downloading, setDownloading] = useState(false)
  const binary = [change.before, change.after].some(
    (entry) =>
      entry &&
      (entry.blob.bytes > 1 << 20 ||
        ['input', 'answer', 'asset', 'resource'].includes(entry.kind) ||
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
    setError('')
    setDownloadError('')
    setError('')
    Promise.all(
      [change.before, change.after].map(async (entry) => {
        if (!entry) return ''
        if (
          entry.blob.bytes > 1 << 20 ||
          ['input', 'answer', 'asset', 'resource'].includes(entry.kind) ||
          entry.attributes.format === 'pdf'
        )
          return `${entry.path}\n${entry.blob.bytes.toLocaleString()} 字节`
        const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256)
        const text = await blob.text()
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
  if (error)
    return (
      <p role="alert" className="text-sm text-destructive">
        {error}
      </p>
    )
  return (
    <div className="min-w-0 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="min-w-0 break-all text-xs text-muted-foreground">
          {change.before?.path || '新增文件'}
          {change.before?.path !== change.after?.path && ` → ${change.after?.path || '已删除'}`}
        </p>
        <div className="flex gap-2">
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
      {binary ? (
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
      <div className="flex flex-wrap justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">
            {history ? '提交历史' : canEdit ? '更改与提交' : '提交差异'}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {history
              ? '每次提交保留一份可比较、可恢复的内容快照。'
              : '保存是私人的；提交后，协作者才能更新到这些更改。'}
          </p>
        </div>
        {history && target && canEdit && (
          <Button variant="outline" loading={busy} onClick={() => void restore()}>
            恢复到工作副本
          </Button>
        )}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {history && (
        <div className="max-h-64 overflow-y-auto rounded-xl border">
          {commits.map((item) => (
            <button
              key={item.revision}
              className={`flex w-full items-center gap-3 border-b px-4 py-3 text-left text-sm last:border-0 hover:bg-muted/40 ${target === item.revision ? 'bg-accent' : ''}`}
              onClick={() => setRevision(item.revision)}
            >
              <span className="w-10 shrink-0 font-mono text-primary">r{item.revision}</span>
              <span className="min-w-0 flex-1 truncate">{item.message}</span>
              <span className="hidden text-xs text-muted-foreground sm:block">
                {formatDateTime(item.createdAt)}
              </span>
            </button>
          ))}
          {!commits.length && <p className="p-6 text-sm text-muted-foreground">还没有提交记录。</p>}
          {hasMore && (
            <div className="border-t p-3">
              <Button
                variant="outline"
                loading={loadingMore}
                disabled={loadingMore}
                onClick={() => void moreHistory()}
              >
                加载更早的提交
              </Button>
            </div>
          )}
        </div>
      )}
      {comparison && (
        <div className="space-y-3">
          <div className="flex flex-wrap gap-2">
            {comparison.changes.map((item) => (
              <Button
                key={item.entryId}
                variant={selected === item.entryId ? 'secondary' : 'outline'}
                onClick={() => setSelected(item.entryId)}
              >
                <span className="font-mono">
                  {item.kind === 'added' ? '+' : item.kind === 'deleted' ? '−' : '~'}
                </span>
                {entryLabel((item.after ?? item.before) as DomainTreeEntry)}
              </Button>
            ))}
          </div>
          {change ? (
            <DiffContent problemId={problemId} change={change} />
          ) : (
            <p className="py-8 text-sm text-muted-foreground">没有未提交的更改。</p>
          )}
        </div>
      )}
      {!history && canEdit && (
        <form
          className="max-w-2xl space-y-3 border-t pt-5"
          noValidate
          onSubmit={(event) => {
            event.preventDefault()
            void commit()
          }}
        >
          <label htmlFor="commit-message" className="text-sm font-medium">
            提交说明
          </label>
          <Textarea
            id="commit-message"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder="例如：补充边界测试并修正题面数据范围"
            maxLength={2000}
          />
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" loading={busy} disabled={!comparison?.changes.length}>
              提交更改
            </Button>
            <span className="text-xs text-muted-foreground">
              提交不会自动发布，也不会触发重测。
            </span>
          </div>
        </form>
      )}
    </div>
  )
}
