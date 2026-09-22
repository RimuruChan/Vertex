import { useRef, useState } from 'react'
import { Check, ChevronDown, Code2, Search, X } from 'lucide-react'
import type { DomainMaterialView } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { entryLabel, roleNames } from '@/lib/authoring-materials'
import { programLanguages, programRoles } from './program-draft'

export default function ProgramNavigation({
  programs,
  selected,
  disabled,
  onSelect,
}: {
  programs: DomainMaterialView[]
  selected: string
  disabled: boolean
  onSelect: (item: DomainMaterialView) => Promise<boolean>
}) {
  const [query, setQuery] = useState('')
  const mobileMenu = useRef<HTMLDetailsElement>(null)
  const current = programs.find((item) => item.entry.id === selected)
  const visible = programs.filter((item) => {
    const language = programLanguages.find(([id]) => id === item.program?.language)?.[1] ?? ''
    return [
      item.program?.name,
      entryLabel(item.entry),
      item.program?.language,
      language,
      roleNames[item.program?.role ?? ''],
    ]
      .join(' ')
      .toLocaleLowerCase()
      .includes(query.trim().toLocaleLowerCase())
  })
  const roles = [
    ...new Set([
      ...programRoles.map((role) => role.id),
      ...programs.map((item) => item.program?.role ?? ''),
    ]),
  ]
  const content = (
    <>
      <div className="border-b p-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="搜索程序"
            placeholder="搜索名称、用途或语言"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="h-9 pl-9 pr-8 text-xs"
          />
          {query && (
            <button
              type="button"
              aria-label="清除程序搜索"
              onClick={() => setQuery('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
        <p className="mt-2 text-[11px] text-muted-foreground" aria-live="polite">
          {query
            ? `找到 ${visible.length} / ${programs.length} 个程序`
            : `${programs.length} 个程序 · 按用途分组`}
        </p>
      </div>
      <nav
        aria-label="题目程序"
        className="max-h-80 space-y-4 overflow-y-auto p-2 lg:max-h-[min(70dvh,760px)]"
      >
        {roles.map((role) => {
          const items = visible.filter((item) => (item.program?.role ?? '') === role)
          if (!items.length) return null
          return (
            <div key={role}>
              <h3 className="mb-1 flex items-center justify-between px-2 text-[11px] font-medium text-muted-foreground">
                <span>{roleNames[role] ?? (role || '待修复')}</span>
                <span>{items.length}</span>
              </h3>
              {items.map((item) => (
                <button
                  key={item.entry.id}
                  type="button"
                  disabled={disabled}
                  aria-current={selected === item.entry.id ? 'page' : undefined}
                  title={item.program?.name ?? entryLabel(item.entry)}
                  onClick={() =>
                    void onSelect(item).then((opened) => {
                      if (opened && mobileMenu.current) mobileMenu.current.open = false
                    })
                  }
                  className={`mt-1 flex w-full min-w-0 items-start gap-2 rounded-lg border px-2.5 py-3 text-left transition-colors disabled:opacity-50 ${selected === item.entry.id ? 'border-primary/25 bg-primary/8 text-primary' : 'border-transparent hover:bg-muted/60'}`}
                >
                  <Code2 className="mt-0.5 size-4 shrink-0" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">
                      {item.program?.name ?? entryLabel(item.entry)}
                    </span>
                    <span className="mt-1 block text-[11px] text-muted-foreground">
                      {programLanguages.find(([id]) => id === item.program?.language)?.[1] ??
                        item.program?.language ??
                        '定义待修复'}
                      {item.program && ` · ${item.program.files.length} 份代码`}
                    </span>
                  </span>
                  {selected === item.entry.id && <Check className="mt-0.5 size-3.5 shrink-0" />}
                </button>
              ))}
            </div>
          )
        })}
        {!visible.length && (
          <div className="px-2 py-6 text-center text-xs text-muted-foreground">
            <p>{query ? '没有匹配的程序' : '新建或导入程序后，会按用途显示在这里。'}</p>
            {query && (
              <Button variant="ghost" size="sm" className="mt-2" onClick={() => setQuery('')}>
                清除搜索
              </Button>
            )}
          </div>
        )}
      </nav>
    </>
  )
  return (
    <aside className="min-w-0 self-start lg:sticky lg:top-[calc(var(--app-header-height)+1rem)]">
      <div className="hidden overflow-hidden rounded-xl border bg-card lg:block">{content}</div>
      <details
        ref={mobileMenu}
        className="group overflow-hidden rounded-xl border bg-card lg:hidden"
      >
        <summary className="flex cursor-pointer list-none items-center gap-3 px-4 py-3 [&::-webkit-details-marker]:hidden">
          <Code2 className="size-4 shrink-0 text-primary" />
          <span className="min-w-0 flex-1">
            <span className="block text-[11px] text-muted-foreground">
              切换程序 · {programs.length} 个
            </span>
            <span className="mt-0.5 block truncate text-sm font-medium">
              {current?.program?.name ?? '浏览全部程序'}
            </span>
          </span>
          <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
        </summary>
        <div className="border-t">{content}</div>
      </details>
    </aside>
  )
}
