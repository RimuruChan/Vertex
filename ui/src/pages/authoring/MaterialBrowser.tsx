import { useEffect, useState } from 'react'
import { ArrowDown, ArrowUp, Trash2, Settings2, Wand2, FileInput } from 'lucide-react'
import type {
  DomainMaterialBatch,
  DomainMaterialPage,
  DomainMaterialView,
  DomainTreeEntry,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Choice, Field } from './MaterialForm'
import { apiError, formatFileSize } from '@/lib/format'
import { entryLabel, materialNames, validationModeName } from '@/lib/authoring-materials'
import { cn } from '@/lib/utils'

export default function MaterialBrowser({
  problemId,
  activeKind,
  hideKindTabs,
  mode,
  copy,
  revision,
  canEdit,
  disabled,
  onSelect,
  onSaved,
  onBusy,
}: {
  problemId: string
  activeKind?: string
  hideKindTabs?: boolean
  mode: 'tests' | 'assets'
  copy: DomainWorkingCopy
  revision?: number
  canEdit: boolean
  disabled: boolean
  onSelect: (entry: DomainTreeEntry) => void
  onSaved: (copy: DomainWorkingCopy) => void
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI(),
    confirm = useConfirm()
  const [kind, setKind] = useState(activeKind ?? (mode === 'tests' ? 'test' : 'raw'))
  const [page, setPage] = useState<DomainMaterialPage>(),
    [cursor, setCursor] = useState(''),
    [previous, setPrevious] = useState<string[]>([])
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [refresh, setRefresh] = useState(0)
  const [checked, setChecked] = useState(new Set<string>()),
    [group, setGroup] = useState(''),
    [groups, setGroups] = useState(new Map<string, string>())
  const [filter, setFilter] = useState('')
  const [batchSettings, setBatchSettings] = useState(false),
    [changeGroup, setChangeGroup] = useState(false)
  const [timeLimit, setTimeLimit] = useState(''),
    [memoryLimit, setMemoryLimit] = useState('')
  const raw = copy.tree.entries.filter((entry) =>
    mode === 'tests'
      ? ['input', 'answer'].includes(entry.kind)
      : mode === 'assets'
        ? ['asset', 'resource'].includes(entry.kind)
        : entry.kind === 'source',
  )
  const files = raw.filter((entry) =>
    entryLabel(entry).toLowerCase().includes(filter.toLowerCase()),
  )
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    if (kind === 'raw') {
      setLoading(false)
      return
    }
    let live = true
    setLoading(true)
    setError('')
    api
      .getApiAuthoringProblemsIdMaterials(problemId, {
        kind,
        limit: 25,
        after: cursor || undefined,
        ...(revision ? { revision } : { etag: copy.etag }),
      })
      .then((value) => {
        if (live) setPage(value)
      })
      .catch((error) => {
        if (live) setError(apiError(error, '材料列表加载失败'))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [api, problemId, kind, cursor, revision, copy.etag, refresh])
  useEffect(() => {
    if (mode !== 'tests') return
    let live = true
    api
      .getApiAuthoringProblemsIdMaterials(problemId, {
        kind: 'group',
        limit: 100,
        ...(revision ? { revision } : { etag: copy.etag }),
      })
      .then((value) => {
        if (live)
          setGroups(
            new Map(
              value.items.map((item) => [
                item.entry.id,
                item.group?.name ?? entryLabel(item.entry),
              ]),
            ),
          )
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [api, problemId, mode, revision, copy.etag])
  const items: DomainMaterialView[] =
    kind === 'raw'
      ? files
          .slice(previous.length * 25, previous.length * 25 + 25)
          .map((entry, index) => ({ entry, position: previous.length * 25 + index + 1 }))
      : (page?.items ?? [])
  const fileLabel = (id?: string) => {
    const file = copy.tree.entries.find((entry) => entry.id === id)
    return file ? formatFileSize(file.blob.bytes) : '尚未配置'
  }
  const hasGroups = copy.tree.entries.some((entry) => entry.kind === 'group')
  const total = kind === 'raw' ? files.length : (page?.total ?? 0)
  const next =
    kind === 'raw'
      ? (previous.length + 1) * 25 < files.length
        ? String((previous.length + 1) * 25)
        : undefined
      : page?.next
  const locked = disabled || busy || loading
  function toggle(id: string) {
    setChecked((current) => {
      const value = new Set(current)
      if (value.has(id)) value.delete(id)
      else value.add(id)
      return value
    })
  }
  async function batch(change: Omit<DomainMaterialBatch, 'etag'>) {
    setBusy(true)
    setError('')
    try {
      const next = await api.postApiAuthoringProblemsIdWorkingCopyBatch(problemId, {
        ...change,
        etag: copy.etag,
      })
      setChecked(new Set())
      setCursor('')
      setPrevious([])
      onSaved(next)
      setBatchSettings(false)
      setRefresh((value) => value + 1)
    } catch (error) {
      setError(apiError(error, '批量操作失败，草稿未改变'))
    } finally {
      setBusy(false)
    }
  }
  async function remove() {
    if (
      await confirm({
        title: `移除 ${checked.size} 份材料？`,
        description:
          '只从工作副本移除所选材料，已有提交和发布版本保留。引用这些材料的程序或测试可能需要调整。',
        confirmLabel: '从副本移除',
        destructive: true,
      })
    )
      await batch({ deleteIds: [...checked] })
  }
  async function move(id: string, direction: number) {
    setBusy(true)
    setError('')
    try {
      const material = await api.getApiAuthoringProblemsIdMaterialsEntryId(problemId, 'problem')
      const order = [...(material.metadata?.testOrder ?? [])],
        index = order.indexOf(id),
        target = index + direction
      if (index < 0 || target < 0 || target >= order.length) return
      ;[order[index], order[target]] = [order[target], order[index]]
      await batch({ testOrder: order })
    } catch (error) {
      setError(apiError(error, '调整顺序失败'))
    } finally {
      setBusy(false)
    }
  }
  const pagination = total > 25 && (
    <div className="flex items-center justify-between gap-3 border-t p-3 text-xs text-muted-foreground">
      <span>
        共 {total} 项 · 第 {previous.length + 1} 页
      </span>
      <div className="flex gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={locked || !previous.length}
          onClick={() => {
            setCursor(previous.at(-1)!)
            setPrevious((value) => value.slice(0, -1))
            setChecked(new Set())
          }}
        >
          上一页
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={locked || !next}
          onClick={() => {
            setPrevious((value) => [...value, cursor])
            setCursor(next!)
            setChecked(new Set())
          }}
        >
          下一页
        </Button>
      </div>
    </div>
  )
  return (
    <div className="space-y-4">
      {mode === 'tests' && !hideKindTabs && (
        <div className="flex flex-wrap gap-2">
          {[
            ['test', '测试点'],
            ['validation', '校验器自测'],
            ['group', '分组'],
            ['raw', '数据文件'],
          ].map(([value, label]) => (
            <Button
              key={value}
              variant={kind === value ? 'secondary' : 'outline'}
              disabled={disabled || busy}
              onClick={() => {
                setKind(value)
                setPage(undefined)
                setCursor('')
                setPrevious([])
                setChecked(new Set())
              }}
            >
              {label}
            </Button>
          ))}
        </div>
      )}
      {error && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 p-3">
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
          <Button variant="outline" size="sm" onClick={() => setRefresh((value) => value + 1)}>
            重试
          </Button>
        </div>
      )}
      {kind === 'raw' && (
        <Input
          aria-label="搜索材料"
          className="max-w-sm"
          value={filter}
          placeholder="按名称搜索…"
          onChange={(event) => {
            setFilter(event.target.value)
            setPrevious([])
            setCursor('')
            setChecked(new Set())
          }}
        />
      )}
      {canEdit && checked.size > 0 && (
        <div className="flex flex-wrap items-end gap-2 rounded-xl border bg-muted/40 p-3">
          <span className="mr-2 self-center text-sm">已选择 {checked.size} 项</span>
          {kind === 'test' && (
            <>
              <Button
                variant="outline"
                size="sm"
                disabled={locked}
                onClick={() => void batch({ testIds: [...checked], patch: { isSample: true } })}
              >
                设为样例
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={locked}
                onClick={() => void batch({ testIds: [...checked], patch: { isSample: false } })}
              >
                设为秘密数据
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={locked}
                onClick={() => {
                  setBatchSettings(true)
                  setChangeGroup(false)
                  setTimeLimit('')
                  setMemoryLimit('')
                }}
              >
                <Settings2 />
                分组与限制
              </Button>
            </>
          )}
          <Button variant="outline" size="sm" disabled={locked} onClick={() => void remove()}>
            <Trash2 />
            移除
          </Button>
          <Button variant="ghost" size="sm" disabled={locked} onClick={() => setChecked(new Set())}>
            取消选择
          </Button>
        </div>
      )}
      <Dialog
        open={batchSettings}
        onOpenChange={(open) => {
          if (!busy) setBatchSettings(open)
        }}
      >
        <DialogContent>
          <DialogTitle>批量调整 {checked.size} 个测试点</DialogTitle>
          <DialogDescription>
            只修改填写的项目，留空的限制保持原值；填 0 恢复题目默认限制。
          </DialogDescription>
          <fieldset disabled={locked} className="space-y-4">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-primary"
                checked={changeGroup}
                onChange={(e) => setChangeGroup(e.target.checked)}
              />
              修改测试组
            </label>
            {changeGroup && (
              <Choice
                label="目标测试组"
                value={group}
                onChange={setGroup}
                options={[
                  ['', '不分组'],
                  ...copy.tree.entries
                    .filter((e) => e.kind === 'group')
                    .map((e) => [e.id, groups.get(e.id) ?? entryLabel(e)] as [string, string]),
                ]}
              />
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="时间限制（ms）">
                <Input
                  aria-label="批量时间限制"
                  type="number"
                  min={0}
                  placeholder="保持原值"
                  value={timeLimit}
                  onChange={(e) => setTimeLimit(e.target.value)}
                />
              </Field>
              <Field label="内存限制（KiB）">
                <Input
                  aria-label="批量内存限制"
                  type="number"
                  min={0}
                  placeholder="保持原值"
                  value={memoryLimit}
                  onChange={(e) => setMemoryLimit(e.target.value)}
                />
              </Field>
            </div>
          </fieldset>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Button variant="outline" disabled={busy} onClick={() => setBatchSettings(false)}>
              取消
            </Button>
            <Button
              loading={busy}
              disabled={locked || (!changeGroup && !timeLimit && !memoryLimit)}
              onClick={() => {
                if (
                  [timeLimit, memoryLimit]
                    .filter((v) => v !== '')
                    .map(Number)
                    .some((v) => !Number.isSafeInteger(v) || v < 0)
                ) {
                  setError('限制必须是非负整数')
                  return
                }
                void batch({
                  testIds: [...checked],
                  patch: {
                    ...(changeGroup ? { group } : {}),
                    ...(timeLimit !== '' ? { timeLimitMs: Number(timeLimit) } : {}),
                    ...(memoryLimit !== '' ? { memoryLimitKb: Number(memoryLimit) } : {}),
                  },
                })
              }}
            >
              应用修改
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      {kind === 'test' && canEdit && (
        <p className="text-xs text-muted-foreground md:hidden">向右滑动可查看排序操作。</p>
      )}
      <div className="overflow-hidden rounded-xl border">
        <div className="max-h-[65dvh] overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 border-b bg-card text-xs text-muted-foreground">
              <tr>
                {canEdit && (
                  <th className="w-10 px-3 py-3">
                    <input
                      type="checkbox"
                      className="size-4 accent-primary"
                      aria-label="选择本页材料"
                      disabled={locked || !items.length}
                      checked={
                        items.length > 0 && items.every((item) => checked.has(item.entry.id))
                      }
                      onChange={(event) =>
                        setChecked(
                          event.target.checked
                            ? new Set(items.map((item) => item.entry.id))
                            : new Set(),
                        )
                      }
                    />
                  </th>
                )}
                <th className="w-14 px-3 py-3 text-left font-normal">
                  {['test', 'validation'].includes(kind) ? '#' : '类型'}
                </th>
                <th className="px-3 py-3 text-left font-normal">名称</th>
                <th className="px-3 py-3 text-left font-normal">
                  {kind === 'test'
                    ? '数据来源'
                    : kind === 'group'
                      ? '计分规则'
                      : kind === 'validation'
                        ? '预期行为'
                        : '大小'}
                </th>
                {kind === 'test' && (
                  <>
                    {hasGroups && <th className="px-3 py-3 text-left font-normal">分组</th>}
                    {canEdit && <th className="px-3 py-3 text-right font-normal">顺序</th>}
                  </>
                )}
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr
                  key={item.entry.id}
                  className={cn(
                    'border-b last:border-0 hover:bg-muted/30',
                    checked.has(item.entry.id) && 'bg-muted/40',
                  )}
                >
                  {canEdit && (
                    <td className="px-3 py-2">
                      <input
                        type="checkbox"
                        className="size-4 accent-primary"
                        aria-label={`选择 ${item.test?.name ?? item.validation?.name ?? item.group?.name ?? entryLabel(item.entry)}`}
                        disabled={locked}
                        checked={checked.has(item.entry.id)}
                        onChange={() => toggle(item.entry.id)}
                      />
                    </td>
                  )}
                  <td className="px-3 py-2 text-xs tabular-nums text-muted-foreground">
                    {['test', 'validation'].includes(kind)
                      ? item.position
                      : materialNames[item.entry.kind]}
                  </td>
                  <td className="px-3 py-2">
                    <button
                      disabled={disabled || busy}
                      onClick={() =>
                        onSelect({
                          ...item.entry,
                          attributes: {
                            ...item.entry.attributes,
                            label:
                              item.test?.name ??
                              item.validation?.name ??
                              item.group?.name ??
                              item.program?.name ??
                              entryLabel(item.entry),
                          },
                        })
                      }
                      className="max-w-[28rem] truncate text-left font-medium hover:text-primary"
                      title={entryLabel(item.entry)}
                    >
                      {item.test?.name ??
                        item.validation?.name ??
                        item.group?.name ??
                        item.program?.name ??
                        entryLabel(item.entry)}
                    </button>
                    {item.test?.isSample && (
                      <span className="ml-2 rounded bg-primary/10 px-1.5 py-0.5 text-xs text-primary">
                        样例
                      </span>
                    )}
                    {item.test?.isPretest && (
                      <span className="ml-2 text-xs text-muted-foreground">预测试</span>
                    )}
                    {item.error && <p className="text-xs text-destructive">文档需修复</p>}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 text-xs text-muted-foreground">
                    {item.test ? (
                      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <span className="inline-flex items-center gap-1.5 text-foreground/80">
                          {item.test.input.kind === 'file' ? (
                            <FileInput className="size-3.5" />
                          ) : (
                            <Wand2 className="size-3.5 text-primary" />
                          )}
                          {item.test.input.kind === 'file' ? '上传 / 编写' : '生成器'}
                        </span>
                        <span>
                          {item.test.input.kind === 'file'
                            ? `输入 ${fileLabel(item.test.input.entry)}`
                            : copy.tree.entries.find(
                                (e) => e.id === item.entry.attributes.generationPlan,
                              )?.attributes.label || '参数化生成'}
                        </span>
                        <span>
                          {item.test.answer.kind === 'file'
                            ? `答案 ${fileLabel(item.test.answer.entry)}`
                            : '标准解生成答案'}
                        </span>
                      </div>
                    ) : item.validation ? (
                      validationModeName(item.validation.mode)
                    ) : item.group ? (
                      item.group.aggregation
                    ) : (
                      formatFileSize(item.entry.blob.bytes)
                    )}
                  </td>
                  {kind === 'test' && (
                    <>
                      {hasGroups && (
                        <td className="px-3 py-2 text-xs text-muted-foreground">
                          {item.test?.group
                            ? (groups.get(item.test.group) ?? item.test.group)
                            : '—'}
                        </td>
                      )}
                      {canEdit && (
                        <td className="px-3 py-2 text-right">
                          <div className="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-8"
                              aria-label={`上移 ${item.test?.name ?? '测试'}`}
                              disabled={locked || item.position === 1}
                              onClick={() => void move(item.entry.id, -1)}
                            >
                              <ArrowUp className="size-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-8"
                              aria-label={`下移 ${item.test?.name ?? '测试'}`}
                              disabled={locked || item.position === total}
                              onClick={() => void move(item.entry.id, 1)}
                            >
                              <ArrowDown className="size-4" />
                            </Button>
                          </div>
                        </td>
                      )}
                    </>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
          {!items.length && (
            <p className="px-5 py-8 text-sm text-muted-foreground">
              {loading ? '加载中…' : '暂无材料。可以添加材料，或从题包导入。'}
            </p>
          )}
        </div>
        {pagination}
      </div>
    </div>
  )
}
