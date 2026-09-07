import { useCallback, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  Archive,
  ArrowRight,
  Check,
  ChevronDown,
  Globe2,
  LockKeyhole,
  Search,
  Triangle,
} from 'lucide-react'
import { getApiDomains } from '@/generated/api/vertex'
import { useOptionalDomain } from './DomainContext'
import { switchDomainPath } from './paths'
import { useAuth } from '@/auth/AuthContext'
import { Link } from './navigation'
import { useRemote } from './useRemote'
import { domainIdentityLabel, switchableDomains } from './switcher'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

export default function DomainSwitcher() {
  const context = useOptionalDomain()
  const [open, setOpen] = useState(false)
  const name = context?.domain.name ?? 'vertex'
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          className="h-10 min-w-0 max-w-44 gap-1.5 px-2 sm:max-w-56 sm:gap-2.5"
          aria-label={
            context
              ? `切换域，当前：${name}${context.domain.archived ? '（已归档，只读）' : ''}`
              : '选择域'
          }
          title={context?.domain.archived ? `${name} · 已归档，只读` : `切换域 · ${name}`}
        >
          <Triangle className="size-5 shrink-0 fill-primary/10 text-primary" strokeWidth={2.5} />
          <span className="min-w-0 truncate font-semibold">{name}</span>
          {context?.domain.archived && (
            <Archive className="size-3 shrink-0 text-muted-foreground" />
          )}
          <ChevronDown className="size-3 shrink-0 text-muted-foreground" />
        </Button>
      </DialogTrigger>
      <DialogContent className="h-[min(32rem,calc(100dvh-2rem))] max-h-none max-w-md gap-0 overflow-hidden p-0">
        <DomainPicker onClose={() => setOpen(false)} />
      </DialogContent>
    </Dialog>
  )
}

function DomainPicker({ onClose }: { onClose: () => void }) {
  const context = useOptionalDomain()
  const { user } = useAuth()
  const location = useLocation()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [keyword, setKeyword] = useState('')
  useEffect(() => {
    const timer = window.setTimeout(() => setKeyword(query.trim()), 200)
    return () => window.clearTimeout(timer)
  }, [query])
  const remote = useRemote(
    useCallback(
      (signal: AbortSignal) => getApiDomains({ size: 100, keyword }, { signal }),
      [keyword, user?.id],
    ),
  )
  const loading = remote.loading || query.trim() !== keyword
  const items = switchableDomains(remote.data?.items ?? [], context?.slug)

  function select(slug: string) {
    onClose()
    if (slug !== context?.slug) navigate(switchDomainPath(slug, location.pathname))
  }

  return (
    <>
      <DialogHeader className="shrink-0 px-5 pb-4 pt-5 pr-12">
        <DialogTitle>切换域</DialogTitle>
        <DialogDescription>
          {context
            ? `当前身份：${domainIdentityLabel(context.domain, user)}${context.domain.archived ? ' · 已归档，只读' : ''}`
            : '选择学习与创作空间，账号在各域通用。'}
        </DialogDescription>
      </DialogHeader>
      <div className="relative mx-4 mb-3 shrink-0">
        <Search className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" />
        <Input
          autoFocus
          aria-label="搜索可进入的域"
          placeholder="搜索名称或标识…"
          className="h-10 pl-9"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </div>
      <div
        className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-2"
        aria-busy={loading}
      >
        {loading ? (
          <p role="status" className="px-3 py-8 text-center text-sm text-muted-foreground">
            正在查找域…
          </p>
        ) : remote.error ? (
          <div className="space-y-3 px-3 py-6 text-center">
            <p role="alert" className="text-sm text-destructive">
              {remote.error}
            </p>
            <Button variant="outline" size="sm" onClick={remote.reload}>
              重试
            </Button>
          </div>
        ) : items.length === 0 ? (
          <p role="status" className="px-3 py-8 text-center text-sm text-muted-foreground">
            没有可切换的匹配域。可以到域目录浏览或申请加入。
          </p>
        ) : (
          <ul className="space-y-1" aria-label="可切换的域">
            {items.map((domain) => {
              const current = domain.slug === context?.slug
              const Icon = domain.official
                ? Triangle
                : domain.visibility === 'private'
                  ? LockKeyhole
                  : Globe2
              return (
                <li key={domain.id}>
                  <button
                    type="button"
                    aria-label={`切换到${domain.name}${domain.archived ? '（已归档）' : ''}`}
                    aria-current={current ? 'true' : undefined}
                    onClick={() => select(domain.slug)}
                    className={cn(
                      'flex min-h-16 w-full items-center gap-3 rounded-lg px-3 py-3 text-left transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
                      current && 'bg-primary/5',
                    )}
                  >
                    <span
                      className={cn(
                        'grid size-9 shrink-0 place-items-center rounded-lg border border-border bg-card text-muted-foreground',
                        current && 'border-primary/15 text-primary',
                      )}
                    >
                      <Icon className="size-4" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span className="truncate text-sm font-medium">{domain.name}</span>
                        {domain.archived && (
                          <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                            已归档
                          </span>
                        )}
                      </span>
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        {domain.slug} · {domainIdentityLabel(domain, user)}
                      </span>
                    </span>
                    {current && (
                      <Check className="size-4 shrink-0 text-primary" aria-label="当前域" />
                    )}
                  </button>
                </li>
              )
            })}
          </ul>
        )}
        {!loading && !remote.error && (remote.data?.total ?? 0) > 100 && (
          <p className="px-3 py-2 text-xs text-muted-foreground">
            仅展示前 100 个匹配域，请输入名称缩小范围。
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center justify-between border-t bg-muted/20 px-5 py-3">
        <span className="text-xs text-muted-foreground">切换保留当前栏目</span>
        <Link
          to="/domains"
          onClick={onClose}
          className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
        >
          浏览与创建域 <ArrowRight className="size-3.5" />
        </Link>
      </div>
    </>
  )
}
