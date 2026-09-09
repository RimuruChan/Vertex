import { Award, Check, Clock3, Snowflake } from 'lucide-react'
import type {
  DtoRankboardCellResponse as RankCell,
  DtoRankboardResponse as Rankboard,
} from '@/generated/api/model'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/misc'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

/** Penalty and solve times are shown in whole contest minutes, as ICPC does. */
function minutes(seconds: number): string {
  return String(Math.floor(seconds / 60))
}

function isScoreFormat(format: string): boolean {
  return format === 'ioi' || format === 'oi'
}

/**
 * One scoreboard square. ICPC shows solve time and failed attempts; IOI and OI
 * show the points earned, because a partial score is the whole point of those
 * formats.
 */
function Cell({ cell, format }: { cell: RankCell; format: string }) {
  const pending = cell.pendingCount > 0
  const solved = Boolean(cell.solvedAt)
  const scoreFormat = isScoreFormat(format)

  if (!solved && !pending && cell.attempts === 0 && cell.score === 0) {
    return (
      <span className="text-border" aria-label="未提交">
        —
      </span>
    )
  }

  return (
    <div className="flex min-h-12 flex-col items-center justify-center gap-1 leading-tight">
      <span
        className={cn(
          'inline-flex min-w-14 items-center justify-center gap-1 rounded-lg px-2 py-1.5 text-sm font-semibold tabular-nums',
          solved && cell.firstSolver && 'bg-verdict-ac text-background',
          solved && !cell.firstSolver && 'bg-verdict-ac-bg text-verdict-ac',
          !solved && cell.score > 0 && 'bg-primary/10 text-primary',
          !solved && cell.score === 0 && cell.attempts > 0 && 'bg-destructive/10 text-destructive',
          pending &&
            !solved &&
            cell.score === 0 &&
            cell.attempts === 0 &&
            'bg-amber-500/15 text-amber-600',
        )}
      >
        {solved ? (
          cell.firstSolver ? (
            <Award className="size-3.5" aria-label="首个通过" />
          ) : (
            <Check className="size-3.5" aria-label="通过" />
          )
        ) : pending ? (
          <Clock3 className="size-3" />
        ) : null}
        {scoreFormat
          ? cell.score
          : solved
            ? minutes(cell.penaltySec)
            : cell.attempts > 0
              ? `−${cell.attempts}`
              : '待定'}
      </span>
      <span className="text-[11px] text-muted-foreground">
        {pending ? `${cell.pendingCount} 份待定` : null}
        {!pending && scoreFormat && cell.attempts > 0 ? `${cell.attempts} 次` : null}
        {!pending && !scoreFormat && solved && cell.attempts > 1 ? `${cell.attempts} 次` : null}
      </span>
    </div>
  )
}

/**
 * The contest scoreboard. The server decides which numbers a viewer is
 * entitled to; this component only renders them, so a frozen board cannot leak
 * jury data through the client.
 */
export default function Scoreboard({
  board,
  highlightUserId,
}: {
  board: Rankboard
  highlightUserId?: string
}) {
  const scoreFormat = isScoreFormat(board.format)
  const problems = board.problems ?? []

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="secondary">{board.format.toUpperCase()}</Badge>
        {board.frozen ? (
          <Badge variant="warning">
            <Snowflake className="size-3" />
            已封榜
          </Badge>
        ) : null}
        {board.juryView ? <Badge variant="secondary">内部实时榜单</Badge> : null}
        <span className="text-xs text-muted-foreground">
          {scoreFormat ? '按总分排名,同分先达到者靠前' : '按通过题数排名,同题数罚时少者靠前'}
        </span>
        <span className="ml-auto text-xs tabular-nums text-muted-foreground">
          {board.rows.length} 位参赛者 · {problems.length} 道题
        </span>
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <Table className="[&_td]:px-3 [&_td]:py-2.5 [&_th]:px-3">
          <TableHeader className="bg-muted">
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-16 text-center">排名</TableHead>
              <TableHead className="sticky left-0 z-10 min-w-40 bg-muted">参赛者</TableHead>
              {scoreFormat ? (
                <TableHead className="w-20 text-right">总分</TableHead>
              ) : (
                <TableHead className="w-16 text-right">通过</TableHead>
              )}
              {!scoreFormat ? (
                <TableHead className="w-24 whitespace-nowrap text-right">罚时 / 分钟</TableHead>
              ) : null}
              {problems.map((problem) => (
                <TableHead
                  key={problem.problemId}
                  className="min-w-24 py-3 text-center"
                  title={problem.title}
                >
                  <div className="flex flex-col items-center gap-0.5">
                    <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-foreground">
                      {problem.color ? (
                        <span
                          className="size-2 rounded-full border border-border"
                          style={{ backgroundColor: problem.color }}
                          aria-hidden
                        />
                      ) : null}
                      {problem.label}
                    </span>
                    {scoreFormat ? (
                      <span className="text-[10px] font-normal text-muted-foreground">
                        {problem.points}
                      </span>
                    ) : null}
                  </div>
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {board.rows.length === 0 ? (
              <TableEmpty colSpan={problems.length + (scoreFormat ? 3 : 4)}>
                <EmptyState title="还没有参赛者" description="报名后会出现在榜单上。" />
              </TableEmpty>
            ) : (
              board.rows.map((row) => (
                <TableRow
                  key={row.userId}
                  className={cn('group', row.userId === highlightUserId && 'bg-primary/5')}
                >
                  <TableCell className="text-center tabular-nums">
                    <span
                      className={cn(
                        'inline-flex size-7 items-center justify-center rounded-full text-xs font-medium',
                        row.rank <= 3 ? 'bg-primary/10 text-primary' : 'text-muted-foreground',
                      )}
                    >
                      {row.rank}
                    </span>
                  </TableCell>
                  <TableCell
                    className={cn(
                      'sticky left-0 z-10 font-medium group-hover:bg-muted',
                      row.userId === highlightUserId ? 'bg-accent' : 'bg-card',
                    )}
                  >
                    {row.username}
                    {row.userId === highlightUserId && (
                      <span className="ml-2 text-xs text-primary">我</span>
                    )}
                    {row.hasPending ? (
                      <Badge variant="warning" className="ml-1.5">
                        待定
                      </Badge>
                    ) : null}
                  </TableCell>
                  {scoreFormat ? (
                    <TableCell className="text-right font-semibold tabular-nums">
                      {row.score}
                    </TableCell>
                  ) : (
                    <TableCell className="text-right font-semibold tabular-nums">
                      {row.solved}
                    </TableCell>
                  )}
                  {!scoreFormat ? (
                    <TableCell className="text-right tabular-nums text-muted-foreground">
                      {minutes(row.penalty)}
                    </TableCell>
                  ) : null}
                  {row.cells.map((cell, index) => (
                    <TableCell key={problems[index]?.problemId ?? index} className="text-center">
                      <Cell cell={cell} format={board.format} />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <Award className="size-3.5 text-verdict-ac" />
          首个通过
        </span>
        <span className="inline-flex items-center gap-1.5">
          <Check className="size-3.5 text-verdict-ac" />
          已通过
        </span>
        <span>−n 未通过尝试</span>
        <span className="inline-flex items-center gap-1.5">
          <Clock3 className="size-3.5" />
          待定结果
        </span>
      </div>
    </div>
  )
}
