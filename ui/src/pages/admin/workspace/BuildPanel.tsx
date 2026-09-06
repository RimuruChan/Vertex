import { CheckCircle2, CircleSlash, Hammer, Loader2, XCircle } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { EmptyState, Progress } from '@/components/ui/misc'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDateTime, formatMemory, formatTime } from '@/lib/format'
import {
  BUILD_STAGE_LABELS,
  BUILD_STATE_LABELS,
  formatBytes,
  isBuildActive,
  type PackageBuild,
} from './types'

function stateBadge(state: string) {
  switch (state) {
    case 'succeeded':
      return <Badge variant="success">{BUILD_STATE_LABELS[state]}</Badge>
    case 'failed':
    case 'dead':
      return <Badge variant="destructive">{BUILD_STATE_LABELS[state]}</Badge>
    case 'running':
      return <Badge variant="warning">{BUILD_STATE_LABELS[state]}</Badge>
    default:
      return <Badge variant="secondary">{BUILD_STATE_LABELS[state] ?? state}</Badge>
  }
}

/**
 * The build console. It mirrors what the worker reports: which stage is
 * running, what each test produced, and how the alternate solutions fared
 * against their declared verdicts.
 */
export default function BuildPanel({
  build,
  issues,
  starting,
  onStart,
  onCancel,
}: {
  build?: PackageBuild
  issues: string[]
  starting: boolean
  onStart: () => void
  onCancel: () => void
}) {
  const active = isBuildActive(build)
  const percent =
    build && build.progressTotal > 0
      ? Math.min(100, Math.round((build.progressDone / build.progressTotal) * 100))
      : 0

  return (
    <div className="flex flex-col gap-4">
      <Card className="flex flex-col gap-3 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-col gap-1">
            <p className="text-sm font-medium">构建题目包</p>
            <p className="text-xs text-muted-foreground">
              在沙箱中依次编译源文件、生成并校验输入、用标程产生答案、自检
              checker,最后发布测试数据。
            </p>
          </div>
          <div className="flex items-center gap-2">
            {active ? (
              <Button variant="outline" onClick={onCancel}>
                <CircleSlash />
                取消构建
              </Button>
            ) : null}
            <Button loading={starting} disabled={active || issues.length > 0} onClick={onStart}>
              <Hammer />
              开始构建
            </Button>
          </div>
        </div>

        {issues.length > 0 ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3">
            <p className="text-sm font-medium text-destructive">还不能构建</p>
            <ul className="mt-1 list-inside list-disc text-sm text-muted-foreground">
              {issues.map((issue) => (
                <li key={issue}>{issue}</li>
              ))}
            </ul>
          </div>
        ) : null}

        {build ? (
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              {stateBadge(build.state)}
              <span className="text-muted-foreground">
                阶段:{BUILD_STAGE_LABELS[build.stage] ?? build.stage}
              </span>
              {build.progressTotal > 0 ? (
                <span className="tabular-nums text-muted-foreground">
                  {build.progressDone} / {build.progressTotal}
                </span>
              ) : null}
              {active ? <Loader2 className="size-4 animate-spin text-muted-foreground" /> : null}
              <span className="ml-auto text-xs text-muted-foreground">
                版本 {build.revision} · {formatDateTime(build.createdAt)}
              </span>
            </div>
            {active ? <Progress value={percent} /> : null}
            {build.errorMessage ? (
              <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
                {build.errorMessage}
              </p>
            ) : null}
          </div>
        ) : null}
      </Card>

      {build?.log ? (
        <Card className="flex flex-col gap-2 p-4">
          <p className="text-sm font-medium">构建日志</p>
          <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-3 font-mono text-xs">
            {build.log}
          </pre>
        </Card>
      ) : null}

      {build && build.solutions.length > 0 ? (
        <Card className="overflow-hidden">
          <p className="border-b border-border p-4 text-sm font-medium">对拍结果</p>
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead>解</TableHead>
                <TableHead className="w-44">预期</TableHead>
                <TableHead className="w-44">实际</TableHead>
                <TableHead className="w-20 text-right">测试点</TableHead>
                <TableHead className="w-24 text-right">最大用时</TableHead>
                <TableHead className="w-24 text-right">最大内存</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {build.solutions.map((solution) => (
                <TableRow key={solution.name}>
                  <TableCell className="font-medium">
                    <div className="flex items-center gap-1.5">
                      {solution.matched ? (
                        <CheckCircle2 className="size-4 text-emerald-500" />
                      ) : (
                        <XCircle className="size-4 text-destructive" />
                      )}
                      {solution.name}
                    </div>
                    {solution.message ? (
                      <div className="truncate text-xs text-muted-foreground">
                        {solution.message}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {solution.expectedVerdict}
                  </TableCell>
                  <TableCell>{solution.actualVerdict}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {solution.failedTest || '—'}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatTime(solution.maxTimeMs)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatMemory(solution.maxMemoryKb)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      ) : null}

      {build && build.tests.length > 0 ? (
        <Card className="overflow-hidden">
          <p className="border-b border-border p-4 text-sm font-medium">测试点结果</p>
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="w-14">#</TableHead>
                <TableHead className="w-24">状态</TableHead>
                <TableHead>来源</TableHead>
                <TableHead className="w-24 text-right">输入</TableHead>
                <TableHead className="w-24 text-right">答案</TableHead>
                <TableHead className="w-24 text-right">标程用时</TableHead>
                <TableHead className="w-24 text-right">标程内存</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {build.tests.map((test) => (
                <TableRow key={test.index}>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {test.index}
                    {test.isSample ? (
                      <Badge variant="success" className="ml-1">
                        样例
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    {test.status === 'ok' ? (
                      <Badge variant="success">通过</Badge>
                    ) : (
                      <Badge variant="destructive">失败</Badge>
                    )}
                  </TableCell>
                  <TableCell className="max-w-0">
                    <div className="truncate font-mono text-xs">{test.command || test.source}</div>
                    {test.message ? (
                      <div className="truncate text-xs text-destructive">{test.message}</div>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatBytes(test.inputBytes)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatBytes(test.answerBytes)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatTime(test.timeMs)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatMemory(test.memoryKb)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      ) : null}

      {!build ? (
        <Card>
          <EmptyState
            icon={<Hammer />}
            title="还没有构建过"
            description="构建会在沙箱里跑一遍完整流程,只有成功的构建才会发布测试数据。"
          />
        </Card>
      ) : null}
    </div>
  )
}
