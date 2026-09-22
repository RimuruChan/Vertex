import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { ArrowUpRight, BookOpen, Code2, Database, Files, Search, Settings2 } from 'lucide-react'
import type { DomainTreeEntry } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { findMaterials, materialCategories, type MaterialCategory } from './material-search'

export default function MaterialFinder({
  open,
  onOpenChange,
  problemId,
  entries,
  etag,
  revision,
  onSelect,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  problemId: string
  entries: DomainTreeEntry[]
  etag: string
  revision?: number
  onSelect: (entry: DomainTreeEntry) => Promise<boolean>
}) {
  const api = useDomainAPI()
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState<MaterialCategory>('全部')
  const [active, setActive] = useState(0)
  const [opening, setOpening] = useState(false)
  const [error, setError] = useState('')
  const lock = useRef(false)
  const input = useRef<HTMLInputElement>(null)
  const options = useRef<(HTMLButtonElement | null)[]>([])
  const listId = useId()
  const indexKey = JSON.stringify([problemId, etag, revision])
  const [names, setNames] = useState<{ key: string; values: Record<string, string> }>()
  const [indexing, setIndexing] = useState(false)
  const [nameError, setNameError] = useState(false)
  const resolvedNames = names?.key === indexKey ? names.values : undefined
  useEffect(() => {
    if (!open || resolvedNames) return
    const controller = new AbortController()
    setIndexing(true)
    setNameError(false)
    const kinds = ['program', 'test', 'group', 'validation', 'generation'].filter((kind) =>
      entries.some((entry) => entry.kind === kind && !entry.attributes.label),
    )
    void Promise.allSettled(
      kinds.map(async (kind) => {
        const values: Record<string, string> = {}
        let after: string | undefined
        do {
          const page = await api.getApiAuthoringProblemsIdMaterials(
            problemId,
            { kind, limit: 100, after, ...(revision ? { revision } : { etag }) },
            { signal: controller.signal },
          )
          if (controller.signal.aborted) return values
          for (const item of page.items) {
            const name =
              item.program?.name ??
              item.test?.name ??
              item.group?.name ??
              item.validation?.name ??
              item.generation?.name
            if (name) values[item.entry.id] = name
          }
          after = page.next || undefined
        } while (after)
        return values
      }),
    ).then((results) => {
      if (controller.signal.aborted) return
      setNames({
        key: indexKey,
        values: Object.assign(
          {},
          ...results.flatMap((result) => (result.status === 'fulfilled' ? [result.value] : [])),
        ),
      })
      setNameError(results.some((result) => result.status === 'rejected'))
      setIndexing(false)
    })
    return () => controller.abort()
  }, [api, open, resolvedNames, indexKey, problemId, etag, revision, entries])
  const matches = useMemo(
    () => findMaterials(entries, problemId, query, category, resolvedNames),
    [entries, problemId, query, category, resolvedNames],
  )
  const visible = matches.slice(0, 60)
  const activeIndex = Math.min(active, Math.max(0, visible.length - 1))
  useEffect(() => {
    if (!open) return
    setQuery('')
    setCategory('全部')
    setActive(0)
    setError('')
  }, [open])
  async function choose(entry: DomainTreeEntry) {
    if (lock.current) return
    lock.current = true
    setOpening(true)
    setError('')
    try {
      if (await onSelect(entry)) onOpenChange(false)
      else setError('当前编辑还未保存。请先返回编辑器处理保存提示，再打开其他材料。')
    } catch {
      setError('暂时无法打开材料，当前输入仍保留。')
    } finally {
      lock.current = false
      setOpening(false)
    }
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!lock.current) onOpenChange(value)
      }}
    >
      <DialogContent
        className="gap-0 overflow-hidden p-0 sm:max-w-2xl"
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          input.current?.focus()
        }}
      >
        <div className="px-5 pb-4 pt-5">
          <DialogTitle>查找题目材料</DialogTitle>
          <DialogDescription className="mt-1">
            按名称或路径定位题面、程序、测试与附件。
          </DialogDescription>
          <div className="relative mt-4">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              ref={input}
              aria-label="查找题目材料"
              placeholder="例如：中文、main.cpp、样例"
              role="combobox"
              aria-autocomplete="list"
              aria-expanded={true}
              aria-controls={listId}
              aria-activedescendant={visible.length ? `${listId}-${activeIndex}` : undefined}
              value={query}
              disabled={opening}
              className="h-11 pl-10"
              onChange={(event) => {
                setQuery(event.target.value)
                setActive(0)
              }}
              onKeyDown={(event) => {
                if (event.nativeEvent.isComposing || opening) return
                if (event.key === 'Enter' && visible[activeIndex]) {
                  event.preventDefault()
                  void choose(visible[activeIndex].entry)
                } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                  event.preventDefault()
                  const next = Math.max(
                    0,
                    Math.min(
                      visible.length - 1,
                      activeIndex + (event.key === 'ArrowDown' ? 1 : -1),
                    ),
                  )
                  setActive(next)
                  options.current[next]?.scrollIntoView({ block: 'nearest' })
                }
              }}
            />
          </div>
          <div className="mt-3 flex flex-wrap gap-1" aria-label="材料类型筛选">
            {materialCategories.map((item) => (
              <Button
                key={item}
                variant={category === item ? 'secondary' : 'ghost'}
                size="sm"
                aria-pressed={category === item}
                disabled={opening}
                onClick={() => {
                  setCategory(item)
                  setActive(0)
                  input.current?.focus()
                }}
              >
                {item}
              </Button>
            ))}
          </div>
        </div>
        {error && (
          <div className="flex flex-wrap items-center gap-2 border-t bg-destructive/5 px-5 py-3">
            <p role="alert" className="flex-1 text-sm text-destructive">
              {error}
            </p>
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
              返回编辑器
            </Button>
          </div>
        )}
        <div
          id={listId}
          role="listbox"
          aria-label="匹配的材料"
          aria-busy={opening}
          className="max-h-[min(52dvh,460px)] overflow-y-auto border-t p-2"
        >
          {visible.map((item, index) => {
            const Icon = {
              题面: BookOpen,
              程序: Code2,
              测试: Database,
              附件: Files,
              设置: Settings2,
              全部: Files,
            }[item.category]
            return (
              <button
                key={item.entry.id}
                id={`${listId}-${index}`}
                ref={(element) => {
                  options.current[index] = element
                }}
                type="button"
                role="option"
                aria-selected={index === activeIndex}
                tabIndex={-1}
                disabled={opening}
                onPointerMove={() => setActive(index)}
                onClick={() => void choose(item.entry)}
                className={`flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left transition-colors disabled:opacity-60 ${index === activeIndex ? 'bg-accent text-accent-foreground' : 'hover:bg-muted/50'}`}
              >
                <Icon className="size-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{item.name}</span>
                  <span className="mt-0.5 block truncate font-mono text-[11px] text-muted-foreground">
                    {item.entry.path}
                  </span>
                </span>
                <span className="hidden text-xs text-muted-foreground sm:inline">
                  {item.category}
                </span>
                <ArrowUpRight className="size-3.5 shrink-0 text-muted-foreground" />
              </button>
            )
          })}
          {!visible.length && (
            <div className="py-10 text-center text-sm text-muted-foreground">
              <p>{indexing ? '正在补充材料名称…' : '没有匹配的材料'}</p>
              <Button
                variant="ghost"
                size="sm"
                className="mt-2"
                onClick={() => {
                  setQuery('')
                  setCategory('全部')
                  setActive(0)
                  input.current?.focus()
                }}
              >
                清除筛选
              </Button>
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t bg-muted/20 px-5 py-3 text-xs text-muted-foreground">
          <span role="status">
            {opening
              ? '正在保存并打开材料…'
              : indexing
                ? '正在补充材料名称，也可按文件名和路径查找'
                : nameError
                  ? '部分名称未能加载，可按文件名和路径查找'
                  : matches.length > 60
                    ? `匹配 ${matches.length} 项，显示前 60 项，请继续输入缩小范围`
                    : `${matches.length} 项材料`}
          </span>
          {nameError && !indexing && (
            <Button
              size="sm"
              variant="ghost"
              disabled={opening}
              onClick={() => {
                setNameError(false)
                setNames(undefined)
              }}
            >
              重新加载名称
            </Button>
          )}
          <span>↑ ↓ 选择 · Enter 打开 · Esc 关闭</span>
        </div>
      </DialogContent>
    </Dialog>
  )
}
