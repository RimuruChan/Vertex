import { useEffect, useRef, useState } from 'react'
import {
  ArrowDown,
  ArrowUp,
  Trash2,
  Settings2,
  Wand2,
  FileInput,
  Search,
  X,
  ListOrdered,
  Plus,
  CheckCircle2,
} from 'lucide-react'
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
import { missingTestMaterials } from './data-review'
import { EmptyState } from '@/components/ui/misc'

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
  onCreate,
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
  onCreate?: () => void
}) {
  const api = useDomainAPI(),
    confirm = useConfirm()
  const [kind, setKind] = useState(activeKind ?? (mode === 'tests' ? 'test' : 'raw'))
  const [page, setPage] = useState<{ key: string; kind: string; value: DomainMaterialPage }>(),
    [cursor, setCursor] = useState(''),
    [previous, setPrevious] = useState<string[]>([])
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [failedRequest, setFailedRequest] = useState(''),
    [refresh, setRefresh] = useState(0)
  const [checked, setChecked] = useState(new Set<string>()),
    [group, setGroup] = useState(''),
    [groups, setGroups] = useState(new Map<string, string>())
  const [filter, setFilter] = useState(''),
    [purpose, setPurpose] = useState('all'),
    [reordering, setReordering] = useState(false),
    [notice, setNotice] = useState('')
  const selectAll = useRef<HTMLInputElement>(null)
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
    entryLabel(entry).toLowerCase().includes(filter.trim().toLowerCase()),
  )
  const requestKey = JSON.stringify([problemId, kind, cursor, revision, copy.etag, refresh])
  const currentPage = page?.key === requestKey ? page.value : undefined
  const stalePage = kind !== 'raw' && (!currentPage || failedRequest === requestKey)
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
    setFailedRequest('')
    api
      .getApiAuthoringProblemsIdMaterials(problemId, {
        kind,
        limit: 25,
        after: cursor || undefined,
        ...(revision ? { revision } : { etag: copy.etag }),
      })
      .then((value) => {
        if (live) setPage({ key: requestKey, kind, value })
      })
      .catch((error) => {
        if (live) {
          setFailedRequest(requestKey)
          setError(apiError(error, '材料列表加载失败'))
        }
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [api, problemId, kind, cursor, revision, copy.etag, refresh, requestKey])
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
  const pageItems: DomainMaterialView[] =
    kind === 'raw'
      ? files
          .slice(previous.length * 25, previous.length * 25 + 25)
          .map((entry, index) => ({ entry, position: previous.length * 25 + index + 1 }))
      : page?.kind === kind
        ? page.value.items
        : []
  const materialLabel = (item: DomainMaterialView) =>
    item.test?.name ??
    item.validation?.name ??
    item.group?.name ??
    item.program?.name ??
    entryLabel(item.entry)
  const missing = new Map(
    pageItems.map((item) => [
      item.entry.id,
      stalePage ? [] : missingTestMaterials(item, copy.tree.entries),
    ]),
  )
  const items =
    kind === 'raw'
      ? pageItems
      : pageItems.filter((item) => {
          if (!materialLabel(item).toLocaleLowerCase().includes(filter.trim().toLocaleLowerCase()))
            return false
          return (
            purpose === 'all' ||
            (purpose === 'sample' && item.test?.isSample) ||
            (purpose === 'judge' && item.test && !item.test.isSample) ||
            (purpose === 'missing' && !!missing.get(item.entry.id)?.length)
          )
        })
  const filtered = Boolean(filter.trim() || purpose !== 'all')
  const allSelected =
    !stalePage && items.length > 0 && items.every((item) => checked.has(item.entry.id))
  useEffect(() => {
    if (selectAll.current) selectAll.current.indeterminate = checked.size > 0 && !allSelected
  }, [checked, allSelected])
  useEffect(() => {
    setChecked(new Set())
  }, [requestKey])
  const fileLabel = (id?: string) => {
    if (stalePage) return '等待刷新'
    const file = copy.tree.entries.find((entry) => entry.id === id)
    return file ? formatFileSize(file.blob.bytes) : '尚未配置'
  }
  const hasGroups = copy.tree.entries.some((entry) => entry.kind === 'group')
  const total = kind === 'raw' ? files.length : (currentPage?.total ?? 0)
  const next =
    kind === 'raw'
      ? (previous.length + 1) * 25 < files.length
        ? String((previous.length + 1) * 25)
        : undefined
      : currentPage?.next
  const locked = disabled || busy || loading || stalePage
  function resetFilters() {
    setFilter('')
    setPurpose('all')
    setChecked(new Set())
    if (kind === 'raw') {
      setPrevious([])
      setCursor('')
    }
  }
  function toggle(id: string) {
    setChecked((current) => {
      const value = new Set(current)
      if (value.has(id)) value.delete(id)
      else value.add(id)
      return value
    })
  }
  async function batch(change: Omit<DomainMaterialBatch, 'etag'>) {
    if (!canEdit || locked) return
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
      setNotice(
        change.testOrder
          ? '测试点顺序已更新'
          : change.deleteIds
            ? `已从草稿移除 ${change.deleteIds.length} 项材料`
            : `已更新 ${change.testIds?.length ?? 0} 个测试点`,
      )
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
    if (!canEdit || locked) return
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
  const pagination = !stalePage && total > 25 && (
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
                setFilter('')
                setPurpose('all')
                setReordering(false)
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
          <div className="flex gap-2">
            {stalePage && cursor && (
              <Button
                variant="ghost"
                size="sm"
                disabled={busy || loading}
                onClick={() => {
                  setCursor('')
                  setPrevious([])
                  setChecked(new Set())
                }}
              >
                返回第一页
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={busy || loading}
              onClick={() => setRefresh((value) => value + 1)}
            >
              重试
            </Button>
          </div>
        </div>
      )}
      {notice && (
        <div
          role="status"
          className="flex items-center justify-between gap-2 rounded-lg bg-primary/5 px-3 py-2 text-sm"
        >
          <span className="flex items-center gap-2">
            <CheckCircle2 className="size-4 text-primary" />
            {notice}
          </span>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            aria-label="关闭操作结果"
            onClick={() => setNotice('')}
          >
            <X />
          </Button>
        </div>
      )}
      <div className="space-y-3 rounded-xl border bg-card p-3 sm:p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="relative w-full sm:max-w-sm">
            <Search className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" />
            <Input
              aria-label={kind === 'raw' ? '搜索材料' : '搜索当前页材料'}
              className="pl-9 pr-9"
              value={filter}
              disabled={busy}
              placeholder={kind === 'raw' ? '按文件名称搜索…' : '搜索当前页名称…'}
              onChange={(event) => {
                setFilter(event.target.value)
                if (kind === 'raw') {
                  setPrevious([])
                  setCursor('')
                }
                setChecked(new Set())
                setReordering(false)
              }}
            />
            {!!filter && (
              <button
                type="button"
                className="absolute right-2 top-2 rounded p-1 text-muted-foreground hover:bg-muted"
                aria-label="清除名称搜索"
                disabled={busy}
                onClick={() => {
                  setFilter('')
                  setChecked(new Set())
                  if (kind === 'raw') {
                    setPrevious([])
                    setCursor('')
                  }
                }}
              >
                <X className="size-4" />
              </button>
            )}
          </div>
          {kind === 'test' && canEdit && (
            <Button
              size="sm"
              variant={reordering ? 'secondary' : 'outline'}
              disabled={locked || filtered || !pageItems.length}
              title={filtered ? '清除筛选后可调整完整测试顺序' : undefined}
              onClick={() => setReordering(!reordering)}
            >
              <ListOrdered />
              {reordering ? '完成排序' : '调整顺序'}
            </Button>
          )}
        </div>
        {kind === 'test' && (
          <div className="flex flex-wrap gap-1.5" aria-label="当前页测试点用途筛选">
            {[
              ['all', '全部', pageItems.length],
              ['sample', '公开样例', pageItems.filter((item) => item.test?.isSample).length],
              [
                'judge',
                '普通评测',
                pageItems.filter((item) => item.test && !item.test.isSample).length,
              ],
              [
                'missing',
                '缺少材料',
                pageItems.filter((item) => missing.get(item.entry.id)?.length).length,
              ],
            ].map(([value, label, count]) => (
              <Button
                key={value}
                size="sm"
                variant={purpose === value ? 'secondary' : 'ghost'}
                aria-pressed={purpose === value}
                disabled={busy || loading || stalePage}
                onClick={() => {
                  setPurpose(String(value))
                  setChecked(new Set())
                  setReordering(false)
                }}
              >
                {label}
                <span className="ml-1 text-xs tabular-nums text-muted-foreground">
                  {loading || stalePage ? '…' : count}
                </span>
              </Button>
            ))}
          </div>
        )}
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
          <p role="status">
            {stalePage
              ? error
                ? '材料更新失败，请重试。上次加载的结果暂不可操作。'
                : '正在更新材料，上次加载的结果暂不可操作…'
              : loading
                ? '正在加载材料…'
                : kind === 'raw'
                  ? `找到 ${total} 份文件，本页显示 ${items.length} 份`
                  : `共 ${total} 项 · 第 ${previous.length + 1} 页显示 ${items.length} / ${pageItems.length} 项`}
            {kind !== 'raw' && (
              <span className="ml-2">
                {kind === 'test' ? '搜索和用途筛选仅作用于当前页' : '名称搜索仅作用于当前页'}
              </span>
            )}
          </p>
          {filtered && (
            <Button size="sm" variant="ghost" disabled={busy} onClick={resetFilters}>
              <X />
              清除筛选
            </Button>
          )}
        </div>
        {purpose === 'missing' && (
          <p className="text-xs text-muted-foreground">
            检查输入、答案、生成器及标准解的直接引用是否存在；不代表数据已通过运行检查。
          </p>
        )}
        {reordering && (
          <p className="text-xs text-muted-foreground">
            使用行末箭头调整完整列表中的顺序。更改会立即保存到草稿。
          </p>
        )}
      </div>
      {canEdit && !stalePage && checked.size > 0 && (
        <div className="sticky top-[calc(var(--app-header-height)+0.75rem)] z-20 flex flex-wrap items-center gap-2 rounded-xl border border-primary/20 bg-card p-3 shadow-sm">
          <span className="mr-2 text-sm font-medium">已选择当前页 {checked.size} 项</span>
          {!allSelected && (
            <Button
              variant="ghost"
              size="sm"
              disabled={locked}
              onClick={() => setChecked(new Set(items.map((item) => item.entry.id)))}
            >
              全选当前可见 {items.length} 项
            </Button>
          )}
          {kind === 'test' && (
            <>
              <Button
                variant="outline"
                size="sm"
                disabled={locked}
                onClick={() => void batch({ testIds: [...checked], patch: { isSample: true } })}
              >
                设为公开样例
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={locked}
                onClick={() => void batch({ testIds: [...checked], patch: { isSample: false } })}
              >
                设为普通评测
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
            移除所选
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
      {kind === 'test' && reordering && (
        <p className="text-xs text-muted-foreground md:hidden">向右滑动可查看排序操作。</p>
      )}
      <div className="overflow-hidden rounded-xl border">
        {canEdit && (
          <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
            <span>勾选后可批量操作，翻页或修改筛选会清空选择。</span>
            <Button
              size="sm"
              variant="ghost"
              disabled={locked || !items.length}
              onClick={() =>
                setChecked(allSelected ? new Set() : new Set(items.map((item) => item.entry.id)))
              }
            >
              {stalePage
                ? '等待最新材料'
                : allSelected
                  ? '取消本页选择'
                  : `全选当前可见 ${items.length} 项`}
            </Button>
          </div>
        )}
        <div className="max-h-[65dvh] overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 border-b bg-card text-xs text-muted-foreground">
              <tr>
                {canEdit && (
                  <th className="w-10 px-3 py-3">
                    <input
                      ref={selectAll}
                      type="checkbox"
                      className="size-4 accent-primary"
                      aria-label="选择当前页筛选后可见的全部材料"
                      disabled={locked || !items.length}
                      checked={allSelected}
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
                    {canEdit && reordering && (
                      <th className="px-3 py-3 text-right font-normal">顺序</th>
                    )}
                  </>
                )}
              </tr>
            </thead>
            <tbody
              aria-busy={stalePage || loading}
              className={stalePage ? 'opacity-50' : undefined}
            >
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
                      disabled={locked}
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
                      className="max-w-[15rem] sm:max-w-[28rem] truncate text-left font-medium hover:text-primary"
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
                    {!!missing.get(item.entry.id)?.length && (
                      <p className="mt-1 text-xs text-verdict-tle">
                        缺少{missing.get(item.entry.id)!.join('、')}
                      </p>
                    )}
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
                          {stalePage
                            ? '等待刷新'
                            : item.test.input.kind === 'file'
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
                          {stalePage
                            ? '等待刷新'
                            : item.test?.group
                              ? (groups.get(item.test.group) ?? item.test.group)
                              : '—'}
                        </td>
                      )}
                      {canEdit && reordering && (
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
          {!items.length &&
            (loading || stalePage ? (
              <p className="px-5 py-10 text-center text-sm text-muted-foreground">
                {error ? '材料列表暂不可用，请重试。' : '正在加载材料…'}
              </p>
            ) : (
              !error && (
                <EmptyState
                  icon={filtered ? <Search /> : <FileInput />}
                  title={
                    filtered
                      ? kind === 'raw'
                        ? '没有匹配的文件'
                        : '当前页没有匹配的材料'
                      : `还没有${kind === 'test' ? '测试点' : kind === 'group' ? '测试组' : kind === 'validation' ? '校验器自测' : '数据文件'}`
                  }
                  description={
                    filtered
                      ? kind === 'raw'
                        ? '试试其他名称，或清除筛选查看全部文件。'
                        : '清除筛选查看本页全部材料，或翻页继续查找。'
                      : '添加材料后，可以在这里检查内容、选择并批量管理。'
                  }
                  action={
                    filtered ? (
                      <Button variant="outline" onClick={resetFilters}>
                        清除筛选
                      </Button>
                    ) : canEdit && onCreate ? (
                      <Button disabled={locked} onClick={onCreate}>
                        <Plus />
                        添加测试点
                      </Button>
                    ) : undefined
                  }
                />
              )
            ))}
        </div>
        {pagination}
      </div>
    </div>
  )
}
