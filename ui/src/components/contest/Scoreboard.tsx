import { Award, Snowflake } from 'lucide-react'
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
    return <span className="text-muted-foreground">·</span>
  }

  return (
    <div className="flex flex-col items-center gap-0.5 leading-tight">
      <span
        className={cn(
          'inline-flex min-w-10 justify-center rounded px-1.5 py-0.5 text-xs font-semibold tabular-nums',
          solved && cell.firstSolver && 'bg-amber-500/20 text-amber-600 dark:text-amber-400',
          solved && !cell.firstSolver && 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
          !solved && cell.score > 0 && 'bg-sky-500/15 text-sky-600 dark:text-sky-400',
          !solved && cell.score === 0 && cell.attempts > 0 && 'bg-destructive/10 text-destructive',
          pending &&
            !solved &&
            cell.score === 0 &&
            cell.attempts === 0 &&
            'bg-amber-500/15 text-amber-600',
        )}
      >
        {scoreFormat ? cell.score : solved ? minutes(cell.penaltySec) : `−${cell.attempts}`}
        {solved && cell.firstSolver ? <Award className="ml-0.5 size-3" /> : null}
      </span>
      <span className="text-[10px] text-muted-foreground">
        {pending ? `+${cell.pendingCount}?` : null}
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
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="secondary">{board.format.toUpperCase()}</Badge>
        {board.frozen ? (
          <Badge variant="warning">
            <Snowflake className="size-3" />
            已封榜
          </Badge>
        ) : null}
        {board.juryView ? <Badge variant="destructive">裁判视图(未封榜)</Badge> : null}
        <span className="text-muted-foreground">
          {scoreFormat ? '按总分排名,同分先达到者靠前' : '按通过题数排名,同题数罚时少者靠前'}
        </span>
      </div>

      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-14">#</TableHead>
              <TableHead className="min-w-40">参赛者</TableHead>
              {scoreFormat ? (
                <TableHead className="w-20 text-right">总分</TableHead>
              ) : (
                <TableHead className="w-16 text-right">通过</TableHead>
              )}
              {!scoreFormat ? <TableHead className="w-20 text-right">罚时</TableHead> : null}
              {problems.map((problem) => (
                <TableHead key={problem.problemId} className="w-20 text-center">
                  <div className="flex flex-col items-center gap-0.5">
                    <span className="inline-flex items-center gap-1 font-semibold">
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
                  className={cn(row.userId === highlightUserId && 'bg-primary/5')}
                >
                  <TableCell className="tabular-nums text-muted-foreground">{row.rank}</TableCell>
                  <TableCell className="font-medium">
                    {row.username}
                    {row.hasPending ? (
                      <Badge variant="warning" className="ml-1.5">
                        ?
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
    </div>
  )
}
