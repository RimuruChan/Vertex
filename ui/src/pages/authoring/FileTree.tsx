import { useMemo, useState } from 'react'
import { ChevronRight, FileCode2, Folder } from 'lucide-react'
import type { DomainTreeEntry } from '@/generated/api/model'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

type Node = { name: string; children: Map<string, Node>; entry?: DomainTreeEntry }
function Branch({
  node,
  selected,
  disabled,
  onSelect,
}: {
  node: Node
  selected?: string
  disabled: boolean
  onSelect: (entry: DomainTreeEntry) => void
}) {
  const [open, setOpen] = useState(false)
  if (node.entry)
    return (
      <button
        disabled={disabled}
        className={cn(
          'flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted',
          selected === node.entry.id && 'bg-accent text-accent-foreground',
        )}
        onClick={() => onSelect(node.entry!)}
        title={node.entry.path}
      >
        <FileCode2 className="size-3.5 shrink-0" />
        <span className="truncate">{node.name}</span>
      </button>
    )
  return (
    <div>
      <button
        className="flex w-full min-w-0 items-center gap-1 rounded-md py-1.5 text-left text-xs hover:bg-muted"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
      >
        <ChevronRight
          className={cn('size-3.5 shrink-0 transition-transform', open && 'rotate-90')}
        />
        <Folder className="mr-1 size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{node.name}</span>
      </button>
      {open && (
        <div className="ml-3 border-l pl-2">
          {[...node.children.values()]
            .sort(
              (a, b) =>
                Number(Boolean(a.entry)) - Number(Boolean(b.entry)) || a.name.localeCompare(b.name),
            )
            .map((child) => (
              <Branch
                key={child.name}
                node={child}
                selected={selected}
                disabled={disabled}
                onSelect={onSelect}
              />
            ))}
        </div>
      )}
    </div>
  )
}
export default function FileTree({
  entries,
  selected,
  disabled,
  onSelect,
}: {
  entries: DomainTreeEntry[]
  selected?: string
  disabled: boolean
  onSelect: (entry: DomainTreeEntry) => void
}) {
  const [search, setSearch] = useState('')
  const roots = useMemo(() => {
    const root: Node = { name: '', children: new Map() }
    for (const entry of entries) {
      let parent = root
      for (const name of entry.path.split('/')) {
        if (!parent.children.has(name)) parent.children.set(name, { name, children: new Map() })
        parent = parent.children.get(name)!
      }
      parent.entry = entry
    }
    return [...root.children.values()].sort((a, b) => a.name.localeCompare(b.name))
  }, [entries])
  const matches = search
    ? entries
        .filter((entry) => entry.path.toLowerCase().includes(search.toLowerCase()))
        .slice(0, 200)
    : []
  return (
    <nav className="space-y-3" aria-label="程序文件树">
      <Input
        aria-label="查找程序文件"
        placeholder="查找文件…"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        className="h-9 text-xs"
      />
      <div className="max-h-[50dvh] overflow-auto">
        {search
          ? matches.map((entry) => (
              <button
                key={entry.id}
                className="block w-full truncate rounded-md px-2 py-2 text-left text-xs hover:bg-muted"
                title={entry.path}
                disabled={disabled}
                onClick={() => onSelect(entry)}
              >
                {entry.path}
              </button>
            ))
          : roots.map((node) => (
              <Branch
                key={node.name}
                node={node}
                selected={selected}
                disabled={disabled}
                onSelect={onSelect}
              />
            ))}
        {search && !matches.length && (
          <p className="py-2 text-xs text-muted-foreground">没有匹配文件</p>
        )}
      </div>
      {search && matches.length === 200 && (
        <p className="text-xs text-muted-foreground">显示前 200 个结果，可缩小搜索范围。</p>
      )}
    </nav>
  )
}
