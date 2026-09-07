import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { RefreshCw } from 'lucide-react'
import { postApiAdminSubmissionsIdRejudge as adminRejudge } from '@/generated/api/vertex'
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
import { useCanonicalResourcePath } from '@/hooks/useCanonicalPath'
import { problemHref } from '@/lib/routes'
import {
  apiError,
  formatDateTime,
  formatMemory,
  formatTime,
  languageLabel,
  shortId,
} from '@/lib/format'
import { cn } from '@/lib/utils'

export default function SubmissionDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { user } = useAuth()
  const toast = useToast()
  const { submission, loading, error, reload, pending } = useSubmission(id)
  useCanonicalResourcePath('submissions', id, submission)
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
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (!submission) {
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
              <Link to="/submissions">返回提交列表</Link>
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
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6">
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

      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center gap-3">
            <VerdictTag status={submission.status} full className="text-sm" />
            <CardTitle className="font-mono text-sm text-muted-foreground">
              #{shortId(submission.id)}
            </CardTitle>
          </div>
          {user?.role === 'admin' ? (
            <Button variant="outline" size="sm" loading={rejudging} onClick={handleRejudge}>
              <RefreshCw />
              重新评测
            </Button>
          ) : null}
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {pending && totalCases > 0 ? (
            <div className="flex flex-col gap-1.5">
              <p className="text-xs text-muted-foreground tabular-nums">
                已评测 {submission.judgedCases} / {totalCases} 个测试点
              </p>
              <Progress value={percent} aria-label="判题进度" />
            </div>
          ) : null}

          <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4 xl:grid-cols-7">
            <Field label="题目">
              <Link to={problemHref(submission)} className="text-primary hover:underline">
                {submission.problemTitle}
              </Link>
            </Field>
            <Field label="提交者">
              <Link to={`/users/${submission.username}`} className="hover:underline">
                {submission.username}
              </Link>
            </Field>
            <Field label="语言">{languageLabel(submission.language)}</Field>
            <Field label="评测版本">v{submission.problemVersion}</Field>
            <Field label="得分">{submission.score}</Field>
            <Field label="用时">{pending ? '—' : formatTime(submission.totalTimeMs)}</Field>
            <Field label="峰值内存">{pending ? '—' : formatMemory(submission.peakMemoryKb)}</Field>
            <Field label="提交时间">{formatDateTime(submission.submittedAt)}</Field>
          </dl>
        </CardContent>
      </Card>

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
      <dd className="truncate font-medium tabular-nums">{children}</dd>
    </div>
  )
}
