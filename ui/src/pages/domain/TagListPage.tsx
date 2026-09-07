import { useCallback, useState } from 'react'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { apiError } from '@/lib/format'

export default function TagListPage() {
  const api = useDomainAPI(),
    { can } = useDomain(),
    navigate = useNavigate(),
    active = useActiveRef(),
    [query, setQuery] = useState(''),
    [name, setName] = useState(''),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null)
  const remote = useRemote(
    useCallback((signal: AbortSignal) => api.getApiAdminTags({ signal }), [api]),
  )
  async function create() {
    if (busy || !name.trim() || !can('domain.resources.manage')) return
    setBusy(true)
    setError(null)
    try {
      const result = await api.postApiAdminTags({ name: name.trim() })
      if (active.current) navigate(`/settings/tags/${result.id}`)
    } catch (cause) {
      if (active.current) setError(apiError(cause, '创建标签失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold">标签目录</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          当前域的分类目录。重命名、合并和删除都在标签详情内进行。
        </p>
      </div>
      {can('domain.resources.manage') && (
        <form
          className="flex max-w-xl gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            void create()
          }}
        >
          <Input
            aria-label="新标签名称"
            required
            placeholder="创建一个标签"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <Button type="submit" loading={busy}>
            创建标签
          </Button>
        </form>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Input
        aria-label="筛选标签"
        placeholder="按名称筛选"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="max-w-xl"
      />
      {remote.loading ? (
        <PageSpinner />
      ) : remote.error ? (
        <EmptyState
          title="标签加载失败"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      ) : (
        <Card className="overflow-hidden">
          <ul className="divide-y">
            {remote.data?.items
              .filter((tag) => tag.name.toLowerCase().includes(query.toLowerCase()))
              .map((tag) => (
                <li key={tag.id}>
                  <Link
                    className="flex items-center justify-between gap-3 p-4 hover:bg-muted/40"
                    to={`/settings/tags/${tag.id}`}
                  >
                    <span className="font-medium">{tag.name}</span>
                    <span className="text-xs text-muted-foreground">
                      {tag.problemCount} 道题目 · 进入详情
                    </span>
                  </Link>
                </li>
              ))}
          </ul>
          {!remote.data?.items.some((tag) =>
            tag.name.toLowerCase().includes(query.toLowerCase()),
          ) && <EmptyState title="没有匹配的标签" />}
        </Card>
      )}
    </div>
  )
}
