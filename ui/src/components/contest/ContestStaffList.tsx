import { useCallback } from 'react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Eye, Gavel } from 'lucide-react'

export default function ContestStaffList({ id, revision }: { id: string; revision: number }) {
  const { getApiContestsIdStaff: listStaff } = useDomainAPI()
  const load = useCallback(
    async (signal: AbortSignal) => {
      const result = await listStaff(id, { signal })
      return result.items
    },
    [id, revision, listStaff],
  )
  const remote = useRemote(load)
  return (
    <Card className="flex flex-col gap-4 rounded-xl p-5 sm:p-6">
      <div>
        <h2 className="text-sm font-semibold">生效赛务名单</h2>
        <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
          下列裁判和观察员包含直接授权及群组继承的结果。修改权限请使用上方授权设置。
        </p>
      </div>
      {remote.loading ? (
        <p role="status" className="text-sm text-muted-foreground">
          正在加载…
        </p>
      ) : remote.error ? (
        <div role="alert">
          <p className="text-sm text-destructive">名单加载失败</p>
          <Button variant="outline" size="sm" onClick={remote.reload}>
            重试
          </Button>
        </div>
      ) : remote.data?.length ? (
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {remote.data.map((member) => (
            <li
              key={member.userId}
              className="flex min-w-0 items-center gap-3 rounded-lg border border-border p-3 text-sm"
            >
              <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
                {member.role === 'jury' ? <Gavel className="size-4" /> : <Eye className="size-4" />}
              </span>
              <div className="min-w-0">
                <span className="block break-words font-medium">{member.username}</span>
                <span className="mt-0.5 block text-xs text-muted-foreground">
                  {member.role === 'jury' ? '裁判' : '观察员'}
                </span>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">暂无通过直接授权或群组继承生效的赛务人员。</p>
      )}
      <p className="text-xs text-muted-foreground">
        比赛负责人和域资源管理者的管理权限独立生效，不列入此名单。
      </p>
    </Card>
  )
}
