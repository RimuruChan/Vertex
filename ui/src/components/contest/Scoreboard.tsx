import { contestFormatName, isScoreContest } from '@/lib/contest-formats'
import { Award, Check, Clock3, Eye, ShieldCheck, Snowflake, Minus, CircleDot } from 'lucide-react'
import type { ReactNode } from 'react'
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
import { MedalBadge, medalLabels } from './MedalBadge'

const firstSolverStyle =
  'bg-[#b9ddc5] text-[#245638] ring-1 ring-inset ring-[#88b89a] dark:bg-verdict-ac dark:text-background dark:ring-verdict-ac'

/** Penalty and solve times are shown in whole contest minutes, as ICPC does. */
function minutes(seconds: number): string {
  return String(Math.floor(seconds / 60))
}

function isScoreFormat(format: string): boolean {
  return isScoreContest(format)
}

/** Compact result cells keep the score/time above attempts or pending results. */
function Cell({ cell, format }: { cell: RankCell; format: string }) {
  const pending = cell.pendingCount > 0
  const solved = Boolean(cell.solvedAt)
  const scoreFormat = isScoreFormat(format)

  if (!solved && !pending && cell.attempts === 0 && cell.score === 0) {
    return (
      <span className="text-muted-foreground/35" aria-label="未提交">
        ·
      </span>
    )
  }

  return (
    <div
      className={cn(
        'mx-auto flex h-10 min-w-16 max-w-24 flex-col items-center justify-center gap-0.5 rounded-md px-1.5 tabular-nums',
        solved && cell.firstSolver && firstSolverStyle,
        solved && !cell.firstSolver && 'bg-verdict-ac-bg/55 text-verdict-ac',
        !solved && !pending && cell.score > 0 && 'bg-primary/8 text-primary',
        !solved &&
          !pending &&
          cell.score === 0 &&
          cell.attempts > 0 &&
          'bg-verdict-wa-bg/45 text-verdict-wa',
        pending && !solved && 'bg-primary/12 text-primary',
      )}
    >
      <span className="inline-flex items-center justify-center gap-1 text-sm font-semibold leading-4">
        {solved ? (
          cell.firstSolver ? (
            <Award className="size-3" aria-label="首个通过" />
          ) : (
            <Check className="size-3" aria-label="通过" />
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
      {(pending || (scoreFormat && cell.attempts > 0) || (solved && cell.attempts > 1)) && (
        <span
          className={cn(
            'whitespace-nowrap text-[10px] leading-3',
            pending
              ? 'text-primary'
              : solved && cell.firstSolver
                ? 'text-[#245638]/80 dark:text-background/85'
                : 'text-muted-foreground',
          )}
        >
          {pending ? `${cell.pendingCount} 待定` : null}
          {!pending && scoreFormat && cell.attempts > 0 ? `${cell.attempts} 次尝试` : null}
          {!pending && !scoreFormat && solved && cell.attempts > 1
            ? `${cell.attempts} 次尝试`
            : null}
        </span>
      )}
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
  toolbar,
}: {
  board: Rankboard
  highlightUserId?: string
  toolbar?: ReactNode
}) {
  const scoreFormat = isScoreFormat(board.format)
  const problems = board.problems ?? []

  return (
    <div className="flex min-w-0 flex-col">
      {toolbar && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border bg-muted/15 px-5 py-3">
          {toolbar}
        </div>
      )}
      <div
        role="group"
        aria-label="榜单信息"
        className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 text-xs"
      >
        <div className="flex h-6 shrink-0 items-center gap-3">
          <span className="font-semibold tracking-wide">{contestFormatName(board.format)}</span>
          <Badge
            variant={board.frozen ? 'warning' : board.juryView ? 'default' : 'secondary'}
            className="h-6 w-22 shrink-0 justify-center gap-1.5 rounded-md px-2 py-0 leading-none"
          >
            {board.frozen ? (
              <Snowflake className="size-3" />
            ) : board.juryView ? (
              <ShieldCheck className="size-3" />
            ) : (
              <Eye className="size-3" />
            )}
            {board.frozen ? '已封榜' : board.juryView ? '内部实时' : '公开榜单'}
          </Badge>
        </div>
        <span className="text-muted-foreground">
          {scoreFormat ? '按总分排名' : '通过题数优先，罚时少者靠前'}
        </span>
        <span className="ml-auto text-xs tabular-nums text-muted-foreground">
          共 {board.rows.length} 位参赛者 · {problems.length} 道题目
        </span>
      </div>

      {board.medals && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-border px-4 py-2.5 text-xs">
          {(['gold', 'silver', 'bronze'] as const).map((medal) => (
            <MedalBadge key={medal} medal={medal}>
              {medalLabels[medal]} {board.medals![medal]}
            </MedalBadge>
          ))}
          <span className="ml-auto text-muted-foreground">
            有效人数 {board.medals.eligible}
            {board.frozen ? ' · 按封榜前可见成绩计算' : ' · 至少通过一题'}
          </span>
        </div>
      )}
      <div className="overflow-hidden border-y border-border">
        <Table aria-label="比赛成绩" className="[&_td]:px-3 [&_td]:py-1.5 [&_th]:px-3">
          <TableHeader className="bg-muted/40">
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-14 text-center">排名</TableHead>
              <TableHead className="sticky left-0 z-10 min-w-40 bg-card">参赛者</TableHead>
              {scoreFormat ? (
                <TableHead className="w-20 text-right">总分</TableHead>
              ) : (
                <TableHead className="w-16 text-right">通过</TableHead>
              )}
              {!scoreFormat ? (
                <TableHead className="w-20 whitespace-nowrap text-right">罚时 / 分钟</TableHead>
              ) : null}
              {problems.map((problem, problemIndex) => (
                <TableHead
                  key={problem.problemId}
                  className="h-12 min-w-20 border-l border-border/40 py-2 text-center"
                  title={problem.title}
                >
                  <div className="flex flex-col items-center gap-0.5">
                    <span className="inline-flex items-center gap-1.5 text-[13px] font-semibold text-foreground">
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
                    ) : (
                      <span className="text-[10px] font-normal text-muted-foreground">
                        {board.rows.filter((row) => row.cells[problemIndex]?.solvedAt).length} 通过
                      </span>
                    )}
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
                  className={cn(
                    'group h-14 border-border/60',
                    row.userId === highlightUserId && 'bg-primary/5',
                  )}
                >
                  <TableCell className="text-center tabular-nums">
                    <div className="flex items-center justify-center gap-1.5">
                      <span
                        className={cn(
                          'inline-flex min-w-6 items-center justify-center text-sm',
                          row.rank <= 3 ? 'font-semibold text-primary' : 'text-muted-foreground',
                        )}
                      >
                        {row.rank}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell
                    className={cn(
                      'sticky left-0 z-10 font-medium group-hover:bg-muted',
                      row.userId === highlightUserId
                        ? 'bg-accent shadow-[inset_3px_0_0_var(--primary)]'
                        : 'bg-card',
                    )}
                  >
                    <div className="flex items-center gap-2">
                      {board.medals && (
                        <span className="inline-flex w-7 shrink-0 justify-center">
                          {row.medal && <MedalBadge medal={row.medal} />}
                        </span>
                      )}
                      <span className="max-w-44 truncate" title={row.username}>
                        {row.username}
                      </span>
                      {row.userId === highlightUserId && (
                        <span className="text-[10px] text-primary">我</span>
                      )}
                      {row.hasPending ? (
                        <Clock3
                          className="size-3 shrink-0 text-primary"
                          aria-label="存在待定结果"
                        />
                      ) : null}
                    </div>
                  </TableCell>
                  {scoreFormat ? (
                    <TableCell className="text-right text-base font-semibold tabular-nums">
                      {row.score}
                    </TableCell>
                  ) : (
                    <TableCell className="text-right text-base font-semibold tabular-nums">
                      {row.solved}
                    </TableCell>
                  )}
                  {!scoreFormat ? (
                    <TableCell className="text-right tabular-nums text-muted-foreground">
                      {minutes(row.penalty)}
                    </TableCell>
                  ) : null}
                  {row.cells.map((cell, index) => (
                    <TableCell
                      key={problems[index]?.problemId ?? index}
                      className="border-l border-border/30 text-center"
                    >
                      <Cell cell={cell} format={board.format} />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-2 bg-muted/10 px-4 py-2.5 text-[11px] text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <span className={cn('grid size-5 place-items-center rounded', firstSolverStyle)}>
            <Award className="size-3" />
          </span>
          首个通过
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="grid size-5 place-items-center rounded bg-verdict-ac-bg/55 text-verdict-ac">
            <Check className="size-3" />
          </span>
          已通过
        </span>
        {scoreFormat ? (
          <span className="inline-flex items-center gap-1.5">
            <span className="grid size-5 place-items-center rounded bg-primary/8 text-primary">
              <CircleDot className="size-3" />
            </span>
            部分得分
          </span>
        ) : (
          <span className="inline-flex items-center gap-1.5">
            <span className="grid size-5 place-items-center rounded bg-verdict-wa-bg/45 text-verdict-wa">
              <Minus className="size-3" />
            </span>
            未通过尝试
          </span>
        )}
        <span className="inline-flex items-center gap-1.5">
          <span className="grid size-5 place-items-center rounded bg-primary/12 text-primary">
            <Clock3 className="size-3" />
          </span>
          待定结果
        </span>
      </div>
    </div>
  )
}
