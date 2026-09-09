import { useState } from 'react'
import { useParams, useLocation } from 'react-router-dom'
import { Link } from '@/domain/navigation'
import { RefreshCw, Clock3, UserRound, Code2 } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useAuth } from '@/auth/AuthContext'
import CodeEditor from '@/components/CodeEditor'
import { CaseStrip } from '@/components/JudgeResultPanel'
import VerdictTag, { verdictStyle } from '@/components/VerdictTag'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState, Progress, Skeleton } from '@/components/ui/misc'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useToast } from '@/components/ui/toast'
import { useSubmission } from '@/hooks/useSubmission'
import { useCanonicalPath } from '@/hooks/useCanonicalPath'
import { problemHref, submissionHref, matchesReference } from '@/lib/routes'
import { useContestSpace } from '@/components/contest/ContestContext'
import { apiError, formatMemory, formatTime, languageLabel, shortId } from '@/lib/format'
import { cn } from '@/lib/utils'

export default function SubmissionDetailPage() {
  const { postApiAdminSubmissionsIdRejudge: adminRejudge } = useDomainAPI()
  const { id, contestId } = useParams()
  const space = useContestSpace()
  const location = useLocation()
  const { user } = useAuth()
  const toast = useToast()
  const { submission, loading, error, reload, pending } = useSubmission(id)
  const matchesContest =
    !contestId ||
    (!!submission &&
      matchesReference(contestId, {
        id: submission.contestId ?? '',
        publicId: submission.contestPublicId,
      }))
  useCanonicalPath(
    submission && matchesContest && matchesReference(id, submission)
      ? submissionHref(submission)
      : undefined,
  )
  const listPath = contestId ? `/contests/${contestId}/submissions` : '/submissions'
  const returnPath =
    typeof location.state?.contestReturn === 'string' &&
    location.state.contestReturn.split('?')[0] === location.pathname.replace(/\/[^/]+$/, '')
      ? location.state.contestReturn
      : listPath
  const [rejudging, setRejudging] = useState(false)

  async function handleRejudge() {
    if (!id) return
    setRejudging(true)
    try {
      await adminRejudge(id)
      toast.success('已加入重测队列')
      await reload()
    } catch (error) {
      toast.error(apiError(error, '重测失败'))
    } finally {
      setRejudging(false)
    }
  }

  if (loading) {
    return (
      <div className="site-container flex flex-col gap-4 py-6">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (!submission || !matchesContest) {
    const notFound = responseStatus(error) === 404
    return (
      <EmptyState
        title={notFound || !error ? '提交记录不存在' : '提交记录加载失败'}
        description={
          notFound || !error
            ? '它可能已被删除,或者你没有查看权限。'
            : apiError(error, '网络暂时不可用,请稍后重试。')
        }
        action={
          <div className="flex flex-wrap justify-center gap-2">
            {!notFound && error ? <Button onClick={() => void reload(true)}>重试</Button> : null}
            <Button variant="outline" asChild>
              <Link to={returnPath}>返回提交列表</Link>
            </Button>
          </div>
        }
      />
    )
  }

  const totalCases = submission.totalCases || submission.caseResults?.length || 0
  const percent =
    totalCases > 0
      ? Math.round(((pending ? submission.judgedCases : totalCases) / totalCases) * 100)
      : 0

  return (
    <div className="site-container flex flex-col gap-4 py-6">
      <Link to={returnPath} className="text-sm text-muted-foreground hover:text-primary">
        ← 返回{contestId ? '本场' : ''}提交记录
      </Link>
      {error ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
        >
          <span>{apiError(error, '最新评测状态获取失败,当前展示的是上次结果。')}</span>
          <Button variant="outline" size="sm" onClick={() => void reload()}>
            重试
          </Button>
        </div>
      ) : null}

      <section
        aria-labelledby="submission-title"
        className="overflow-hidden rounded-xl border border-border bg-card"
      >
        <div className="flex flex-col gap-5 p-5 sm:p-6">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 flex-1">
              <p className="mb-2 text-xs font-medium text-muted-foreground">
                提交 #{submission.publicId || shortId(submission.id)}
              </p>
              <h1
                id="submission-title"
                className="text-xl font-semibold tracking-tight break-words sm:text-2xl"
              >
                <Link
                  to={problemHref({
                    ...submission,
                    label: space?.details?.problems.find(
                      (item) => item.problemId === submission.problemId,
                    )?.label,
                  })}
                  className="hover:text-primary"
                >
                  {submission.problemTitle || '提交详情'}
                </Link>
              </h1>
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-3">
              <VerdictTag status={submission.status} full className="px-3 py-1.5 text-sm" />
              {(contestId
                ? space?.details?.contest.permissions.rejudge
                : user?.role === 'admin') && (
                <Button variant="ghost" size="sm" loading={rejudging} onClick={handleRejudge}>
                  <RefreshCw />
                  重新评测
                </Button>
              )}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-x-5 gap-y-3 text-sm text-muted-foreground">
            <span className="inline-flex items-center gap-2">
              <UserRound className="size-4 shrink-0" />
              <Link
                to={
                  contestId
                    ? `/contests/${contestId}/submissions?user=${encodeURIComponent(submission.username || submission.userId || '')}&mine=0`
                    : `/users/${submission.username}`
                }
                className="break-all hover:text-primary"
              >
                {submission.username || '未知用户'}
              </Link>
            </span>
            <span className="inline-flex items-center gap-2">
              <Code2 className="size-4 shrink-0" />
              {languageLabel(submission.language)}
              <span className="text-border">/</span>评测版本 v{submission.problemVersion}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
            <Clock3 className="size-4 shrink-0 text-muted-foreground" />
            <span className="text-muted-foreground">提交于</span>
            <time
              dateTime={submission.submittedAt}
              title={submission.submittedAt}
              className="font-medium tabular-nums"
            >
              {new Date(submission.submittedAt).toLocaleString(undefined, {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
                hour12: false,
              })}
            </time>
          </div>
        </div>
        <dl className="grid grid-cols-3 gap-4 border-t border-border bg-background/50 px-5 py-4 sm:px-6">
          <Field label="得分">
            {pending || submission.status === 'Submitted' ? '—' : submission.score}
          </Field>
          <Field label="用时">
            {pending || submission.status === 'Submitted'
              ? '—'
              : formatTime(submission.totalTimeMs)}
          </Field>
          <Field label="峰值内存">
            {pending || submission.status === 'Submitted'
              ? '—'
              : formatMemory(submission.peakMemoryKb)}
          </Field>
        </dl>
        {pending && totalCases > 0 && (
          <div className="flex flex-col gap-2 border-t border-border px-5 py-4 sm:px-6">
            <p className="text-xs text-muted-foreground tabular-nums">
              已评测 {submission.judgedCases} / {totalCases} 个测试点
            </p>
            <Progress value={percent} aria-label="判题进度" />
          </div>
        )}
      </section>

      {submission.status === 'Compile Error' && submission.compileResult ? (
        <Card>
          <CardHeader>
            <CardTitle>编译信息</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="overflow-x-auto rounded-md border border-border bg-muted p-3 font-mono text-xs whitespace-pre-wrap">
              {submission.compileResult}
            </pre>
          </CardContent>
        </Card>
      ) : null}

      {submission.caseResults?.length ? (
        <Card>
          <CardHeader>
            <CardTitle>测试点结果</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <CaseStrip submission={submission} />
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-20">测试点</TableHead>
                  <TableHead className="w-28">判定</TableHead>
                  <TableHead className="w-24 text-right">用时</TableHead>
                  <TableHead className="w-28 text-right">内存</TableHead>
                  <TableHead>详情</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {submission.caseResults.map((item) => (
                  <TableRow key={item.caseIndex}>
                    <TableCell className="font-mono text-xs">#{item.caseIndex}</TableCell>
                    <TableCell>
                      <span
                        className={cn(
                          'rounded-md px-2 py-0.5 text-xs font-medium',
                          verdictStyle(item.verdict).className,
                        )}
                      >
                        {verdictStyle(item.verdict).short}
                      </span>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatTime(item.timeMs)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMemory(item.memoryKb)}
                    </TableCell>
                    <TableCell className="min-w-64 align-top text-xs text-muted-foreground">
                      <CaseDiagnostics
                        checkerOutput={item.checkerOutput}
                        exitStatus={item.exitStatus}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      ) : null}

      {submission.sourceCode ? (
        <Card>
          <CardHeader>
            <CardTitle>源代码</CardTitle>
            <span className="text-xs text-muted-foreground">
              {languageLabel(submission.language)}
            </span>
          </CardHeader>
          <CardContent>
            <div className="h-96">
              <CodeEditor value={submission.sourceCode} language={submission.language} readOnly />
            </div>
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}

function responseStatus(error: unknown): number | undefined {
  return (error as { response?: { status?: number } } | null)?.response?.status
}

function CaseDiagnostics({
  checkerOutput,
  exitStatus,
}: {
  checkerOutput?: string
  exitStatus?: string
}) {
  if (!checkerOutput && !exitStatus) return <span>—</span>

  return (
    <details>
      <summary className="w-fit cursor-pointer rounded-sm text-foreground underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        查看诊断
      </summary>
      <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted p-3 font-mono text-xs text-foreground">
        {exitStatus ? `退出状态: ${exitStatus}` : ''}
        {exitStatus && checkerOutput ? '\n\n' : ''}
        {checkerOutput ? `Checker 输出:\n${checkerOutput}` : ''}
      </pre>
    </details>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-base font-semibold tabular-nums break-words sm:text-lg">
        {children}
      </dd>
    </div>
  )
}
