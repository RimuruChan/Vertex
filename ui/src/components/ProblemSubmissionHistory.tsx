import { useEffect, useState } from 'react'
import { ExternalLink } from 'lucide-react'
import { useAuth } from '@/auth/AuthContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Link } from '@/domain/navigation'
import type { DtoSubmissionResponse } from '@/generated/api/model'
import VerdictTag from './VerdictTag'
import { Button } from './ui/button'
import { EmptyState, Skeleton } from './ui/misc'
import { Pagination } from './ui/pagination'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table'
import { apiError, formatDateTime, formatRelative, languageLabel, shortId } from '@/lib/format'
import { submissionHref } from '@/lib/routes'

const PAGE_SIZE = 15

export default function ProblemSubmissionHistory({
  problemId,
  contestId,
  readAll = false,
}: {
  problemId: string
  contestId?: string
  readAll?: boolean
}) {
  const { getApiSubmissions: listSubmissions } = useDomainAPI()
  const { user } = useAuth()
  const [mine, setMine] = useState(true)
  const [page, setPage] = useState(1)
  const [retry, setRetry] = useState(0)
  const [items, setItems] = useState<DtoSubmissionResponse[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    if (!user) return
    const controller = new AbortController()
    let timer: number | undefined
    setLoading(true)
    setError(null)
    setItems([])
    const load = async () => {
      try {
        const result = await listSubmissions(
          {
            problem: problemId,
            contest: contestId,
            user: !readAll || mine ? user.username : undefined,
            page,
            size: PAGE_SIZE,
          },
          { signal: controller.signal },
        )
        if (!controller.signal.aborted) {
          setItems(result.items)
          setTotal(result.total)
          setError(null)
        }
      } catch (cause) {
        if (!controller.signal.aborted) setError(apiError(cause, '历史提交加载失败'))
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false)
          timer = window.setTimeout(() => {
            void load()
          }, 4000)
        }
      }
    }
    void load()
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [problemId, contestId, user?.id, user?.username, mine, readAll, page, retry, listSubmissions])

  if (!user) return <EmptyState title="登录后查看历史提交" />
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-sm font-semibold">历史提交</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {contestId ? '本场比赛的当前题目' : '当前题目'} · {total} 条
          </p>
        </div>
        {readAll && (
          <div
            role="group"
            aria-label="历史提交范围"
            className="flex gap-1 rounded-md bg-muted p-1"
          >
            <Button
              size="sm"
              variant={mine ? 'ghost' : 'secondary'}
              aria-pressed={!mine}
              onClick={() => {
                setMine(false)
                setPage(1)
              }}
            >
              全部
            </Button>
            <Button
              size="sm"
              variant={mine ? 'secondary' : 'ghost'}
              aria-pressed={mine}
              onClick={() => {
                setMine(true)
                setPage(1)
              }}
            >
              我的
            </Button>
          </div>
        )}
      </div>
      {error && (
        <div
          role="alert"
          className="flex items-center justify-between gap-2 text-sm text-destructive"
        >
          <span>{error}</span>
          <Button size="sm" variant="outline" onClick={() => setRetry((value) => value + 1)}>
            重试
          </Button>
        </div>
      )}
      {loading ? (
        <div role="status" aria-label="正在加载历史提交" className="space-y-3">
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
        </div>
      ) : items.length ? (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>提交</TableHead>
                <TableHead>判定</TableHead>
                {readAll && !mine && <TableHead>提交者</TableHead>}
                <TableHead>语言</TableHead>
                <TableHead className="text-right">时间</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell>
                    <Link
                      to={submissionHref(item)}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={`在新标签页查看提交 #${item.publicId || shortId(item.id)}`}
                      className="inline-flex items-center gap-1 font-mono text-xs text-primary hover:underline"
                    >
                      #{item.publicId || shortId(item.id)}
                      <ExternalLink className="size-3" />
                    </Link>
                  </TableCell>
                  <TableCell>
                    <VerdictTag status={item.status} />
                  </TableCell>
                  {readAll && !mine && <TableCell>{item.username}</TableCell>}
                  <TableCell className="whitespace-nowrap text-xs">
                    {languageLabel(item.language)}
                  </TableCell>
                  <TableCell className="text-right whitespace-nowrap text-xs text-muted-foreground">
                    <time dateTime={item.submittedAt} title={formatDateTime(item.submittedAt)}>
                      {formatRelative(item.submittedAt)}
                    </time>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Pagination page={page} total={total} size={PAGE_SIZE} onChange={setPage} />
        </>
      ) : (
        !error && (
          <EmptyState title="暂无历史提交" description="提交代码后，记录会自动出现在这里。" />
        )
      )}
    </div>
  )
}
