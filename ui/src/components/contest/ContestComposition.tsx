import { useCallback, useEffect, useState } from 'react'
import { ArrowDown, ArrowUp, Plus, Search, Trash2, Check, BookOpen } from 'lucide-react'
import type {
  DtoContestResponse,
  DtoContestProblemResponse,
  DtoProblemResponse,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { SaveButton } from '@/components/ui/save-button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError } from '@/lib/format'
import { compositionPayload, nextProblemLabel } from './contest-form'

const presetColors = [
  '#ef4444',
  '#f97316',
  '#f59e0b',
  '#eab308',
  '#84cc16',
  '#22c55e',
  '#14b8a6',
  '#06b6d4',
  '#3b82f6',
  '#6366f1',
  '#a855f7',
  '#ec4899',
  '#a16207',
  '#64748b',
]

export default function ContestComposition({
  contest,
  problems,
  canEdit,
  onSaved,
}: {
  contest: DtoContestResponse
  problems: DtoContestProblemResponse[]
  canEdit: boolean
  onSaved: () => void
}) {
  const api = useDomainAPI(),
    active = useActiveRef(),
    confirm = useConfirm()
  const [entries, setEntries] = useState(() => problems.map((p) => ({ ...p }))),
    [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [page, setPage] = useState(1),
    [dirty, setDirty] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null)
  const load = useCallback(
    async (signal: AbortSignal) =>
      canEdit
        ? api.getApiProblems({ keyword, page, size: 10, view: 'available' }, { signal })
        : { items: [], total: 0 },
    [api, canEdit, keyword, page],
  )
  const choices = useRemote(load)
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    if (!dirty) setEntries(problems.map((problem) => ({ ...problem })))
    // A settings/permission refresh must not overwrite unsaved composition.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [problems])
  function replace(next: DtoContestProblemResponse[]) {
    setEntries(next)
    setDirty(true)
    setSaved(false)
  }
  function change(index: number, field: 'label' | 'color' | 'points', value: string | number) {
    replace(entries.map((entry, i) => (i === index ? { ...entry, [field]: value } : entry)))
  }
  function add(problem: DtoProblemResponse) {
    if (!canEdit || !problem.publishedVersion || entries.some((p) => p.problemId === problem.id))
      return
    replace([
      ...entries,
      {
        contestId: contest.id,
        contestPublicId: contest.publicId,
        problemId: problem.id,
        problemPublicId: problem.publicId,
        title: problem.title,
        version: problem.publishedVersion,
        difficulty: problem.difficulty,
        visibility: problem.visibility,
        tags: problem.tags,
        sortOrder: entries.length,
        label: nextProblemLabel(entries.map((p) => p.label)),
        color: '',
        points: 100,
      },
    ])
  }
  function move(index: number, delta: number) {
    const next = [...entries],
      target = index + delta
    if (target < 0 || target >= entries.length) return
    ;[next[index], next[target]] = [next[target], next[index]]
    replace(next)
  }
  async function save() {
    if (!canEdit || busy || !dirty) return
    let payload
    try {
      payload = compositionPayload(entries)
    } catch (cause) {
      setError((cause as Error).message)
      return
    }
    if (
      !(await confirm({
        title: '保存比赛题目编排？',
        description:
          '顺序、题号与分值会更新并重算榜单。已有题目仍固定原发布版本，不会自动采用新版；移出的题目不再显示在当前榜单。',
        confirmLabel: '保存编排',
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    setSaved(false)
    setError(null)
    try {
      await api.putApiAdminContestsIdProblems(contest.id, payload)
      if (!active.current) return
      setDirty(false)
      setSaved(true)
      onSaved()
    } catch (cause) {
      if (active.current) setError(apiError(cause, '题目编排保存失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">题目编排</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {canEdit
              ? '从右侧加入题目，再调整题号、顺序与分值。修改完成后统一保存。'
              : '当前为只读，比赛进行中的编排调整由负责人或域管理员操作。'}
          </p>
        </div>
        {canEdit && (
          <Button variant="outline" size="sm" className="lg:hidden" asChild>
            <a href="#contest-problem-picker">
              <Plus />
              加入题目
            </a>
          </Button>
        )}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      <div className={canEdit ? 'grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_300px]' : ''}>
        <Card className="min-w-0 overflow-hidden rounded-xl">
          <div className="flex items-center justify-between gap-3 border-b border-border px-5 py-4">
            <div className="flex items-center gap-2">
              <BookOpen className="size-4 text-primary" />
              <h3 className="text-sm font-semibold">已选题目</h3>
              <span className="rounded bg-muted px-2 py-0.5 text-xs tabular-nums">
                {entries.length}
              </span>
            </div>
            {contest.format !== 'icpc' && (
              <span className="text-xs text-muted-foreground tabular-nums">
                总分 {entries.reduce((sum, item) => sum + item.points, 0)}
              </span>
            )}
          </div>
          {entries.length ? (
            <ol className="divide-y divide-border">
              {entries.map((entry, index) => (
                <li key={entry.problemId} className="p-4 sm:p-5">
                  <div className="mb-4 flex items-start gap-3">
                    <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-primary/10 font-mono text-sm font-semibold text-primary">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <div className="min-w-0 flex-1">
                      <h4 className="break-words text-sm font-semibold">{entry.title}</h4>
                      <p className="mt-1 text-xs text-muted-foreground">
                        #{entry.problemPublicId} · 发布版本 v{entry.version}
                      </p>
                    </div>
                    {canEdit && (
                      <div className="flex shrink-0 gap-0.5">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`上移 ${entry.label}`}
                          disabled={busy || index === 0}
                          onClick={() => move(index, -1)}
                        >
                          <ArrowUp />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`下移 ${entry.label}`}
                          disabled={busy || index === entries.length - 1}
                          onClick={() => move(index, 1)}
                        >
                          <ArrowDown />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                          aria-label={`移除 ${entry.label}`}
                          disabled={busy}
                          onClick={() =>
                            replace(entries.filter((p) => p.problemId !== entry.problemId))
                          }
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    )}
                  </div>
                  <fieldset
                    disabled={!canEdit || busy}
                    className="grid gap-3 sm:grid-cols-[72px_100px_minmax(0,1fr)]"
                  >
                    <label className="flex flex-col gap-1.5 text-xs text-muted-foreground">
                      题号
                      <Input
                        aria-label={`${entry.problemPublicId} 题号`}
                        maxLength={8}
                        value={entry.label}
                        onChange={(e) => change(index, 'label', e.target.value)}
                      />
                    </label>
                    <label className="flex flex-col gap-1.5 text-xs text-muted-foreground">
                      分值
                      <Input
                        aria-label={`${entry.problemPublicId} 分值`}
                        type="number"
                        min={1}
                        max={100000}
                        value={entry.points}
                        onChange={(e) => change(index, 'points', Number(e.target.value))}
                      />
                    </label>
                    <div className="flex min-w-0 max-w-xs flex-col gap-1.5">
                      <label
                        htmlFor={`color-${entry.problemId}`}
                        className="text-xs text-muted-foreground"
                      >
                        榜单颜色
                      </label>
                      <div className="flex items-center gap-2">
                        <Input
                          id={`color-${entry.problemId}`}
                          aria-label={`${entry.problemPublicId} 颜色`}
                          maxLength={32}
                          placeholder="默认"
                          value={entry.color}
                          onChange={(e) => change(index, 'color', e.target.value)}
                          className="min-w-0 flex-1"
                        />
                        <div className="grid shrink-0 grid-cols-7 gap-1">
                          {presetColors.map((color) => (
                            <button
                              key={color}
                              type="button"
                              title={color}
                              aria-label={`${entry.label} 使用颜色 ${color}`}
                              aria-pressed={entry.color === color}
                              onClick={() =>
                                change(index, 'color', entry.color === color ? '' : color)
                              }
                              className="grid size-5 shrink-0 place-items-center rounded-full border border-black/10 text-white focus-visible:outline-2 focus-visible:outline-primary disabled:opacity-40"
                              style={{ backgroundColor: color }}
                            >
                              {entry.color === color && <Check className="size-3" />}
                            </button>
                          ))}
                        </div>
                      </div>
                    </div>
                  </fieldset>
                </li>
              ))}
            </ol>
          ) : (
            <div className="flex flex-col items-center gap-2 px-5 py-16 text-center">
              <BookOpen className="mb-1 size-8 text-muted-foreground/50" />
              <p className="text-sm font-medium">还没有加入题目</p>
              <p className="text-xs text-muted-foreground">从选题区搜索并加入已发布的题目。</p>
            </div>
          )}
          <p className="border-t border-border bg-muted/20 px-5 py-3 text-xs leading-relaxed text-muted-foreground">
            调整顺序不会改变题号。已有题目保留当前发布版本，更新版本请使用赛务操作。
          </p>
        </Card>
        {canEdit && (
          <Card
            id="contest-problem-picker"
            className="flex min-w-0 scroll-mt-32 flex-col gap-4 rounded-xl p-4 lg:sticky lg:top-32"
          >
            <div>
              <h3 className="text-sm font-semibold">加入题目</h3>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                选择有权访问的本域已发布题目。
              </p>
            </div>
            <form
              className="flex gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                setPage(1)
                setKeyword(query.trim())
              }}
            >
              <Input
                aria-label="搜索可加入题目"
                placeholder="搜索题目名称或来源"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <Button type="submit" variant="outline" size="icon" aria-label="搜索题目">
                <Search />
              </Button>
            </form>
            {choices.loading ? (
              <p role="status" className="py-6 text-center text-sm text-muted-foreground">
                正在查找题目…
              </p>
            ) : choices.error ? (
              <div>
                <p role="alert" className="text-sm text-destructive">
                  {choices.error}
                </p>
                <Button variant="outline" size="sm" onClick={choices.reload}>
                  重试
                </Button>
              </div>
            ) : (
              <>
                {choices.data?.items.length ? (
                  <ul className="max-h-[50vh] divide-y divide-border overflow-y-auto">
                    {choices.data.items.map((problem) => {
                      const included = entries.some((p) => p.problemId === problem.id)
                      return (
                        <li key={problem.id} className="flex items-start gap-2 py-3">
                          <div className="min-w-0 flex-1">
                            <p className="break-words text-sm font-medium">{problem.title}</p>
                            <p className="mt-1 text-xs text-muted-foreground">
                              #{problem.publicId} ·{' '}
                              {problem.publishedVersion
                                ? `v${problem.publishedVersion}`
                                : '尚未发布'}
                            </p>
                          </div>
                          <Button
                            variant={included ? 'ghost' : 'outline'}
                            size="icon-sm"
                            aria-label={
                              included ? `${problem.title} 已加入` : `加入 ${problem.title}`
                            }
                            disabled={busy || !problem.publishedVersion || included}
                            onClick={() => add(problem)}
                          >
                            {included ? <Check /> : <Plus />}
                          </Button>
                        </li>
                      )
                    })}
                  </ul>
                ) : (
                  <p className="py-5 text-center text-sm text-muted-foreground">没有匹配的题目。</p>
                )}
                <Pagination
                  page={page}
                  size={10}
                  total={choices.data?.total ?? 0}
                  onChange={setPage}
                />
              </>
            )}
          </Card>
        )}
      </div>
      {canEdit && (
        <div className="sticky bottom-4 z-20 mx-auto flex w-fit max-w-full flex-wrap items-center justify-center gap-x-6 gap-y-2 rounded-2xl border border-border bg-card/95 px-4 py-2.5 shadow-lg backdrop-blur-sm">
          <span role="status" className="text-sm text-muted-foreground">
            {dirty ? '有未保存修改' : `已保存 ${entries.length} 道题目`}
          </span>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              disabled={busy || !dirty}
              onClick={() => {
                setEntries(problems.map((p) => ({ ...p })))
                setDirty(false)
                setError(null)
              }}
            >
              撤销修改
            </Button>
            <SaveButton onClick={() => void save()} loading={busy} saved={saved} disabled={!dirty}>
              保存编排
            </SaveButton>
          </div>
        </div>
      )}
    </div>
  )
}
