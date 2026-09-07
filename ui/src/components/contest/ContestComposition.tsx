import { useCallback, useEffect, useState } from 'react'
import { ArrowDown, ArrowUp, Plus, Search, Trash2 } from 'lucide-react'
import type {
  DtoContestResponse,
  DtoContestProblemResponse,
  DtoProblemResponse,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { useToast } from '@/components/ui/toast'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError } from '@/lib/format'
import { compositionPayload, nextProblemLabel } from './contest-form'

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
    toast = useToast(),
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
        ? api.getApiProblems({ keyword, page, size: 10 }, { signal })
        : { items: [], total: 0 },
    [api, canEdit, keyword, page],
  )
  const choices = useRemote(load)
  useEffect(() => {
    if (!dirty) setEntries(problems.map((problem) => ({ ...problem })))
    // A settings/permission refresh must not overwrite unsaved composition.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [problems])
  function replace(next: DtoContestProblemResponse[]) {
    setEntries(next)
    setDirty(true)
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
    setError(null)
    try {
      await api.putApiAdminContestsIdProblems(contest.id, payload)
      if (!active.current) return
      setDirty(false)
      toast.success('题目编排已保存')
      onSaved()
    } catch (cause) {
      if (active.current) setError(apiError(cause, '题目编排保存失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <Card className="space-y-4 p-4 sm:p-5">
        <div>
          <h2 className="font-medium">题目编排</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            题号、分值、颜色随题目保留，移动顺序不会重新编号。现有发布版本固定；采用新版请通过赛务控制台操作。
          </p>
        </div>
        {!canEdit && (
          <p className="text-sm text-muted-foreground">
            当前仅可查看。编辑协作者开赛后不能调整编排。
          </p>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {entries.length ? (
          <ol className="divide-y">
            {entries.map((entry, index) => (
              <li key={entry.problemId} className="space-y-3 py-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="min-w-0 break-words text-sm font-medium">
                    <span className="mr-2 font-mono text-muted-foreground">
                      {entry.problemPublicId}
                    </span>
                    {entry.title}{' '}
                    <span className="font-normal text-muted-foreground">· v{entry.version}</span>
                  </p>
                  {canEdit && (
                    <div className="flex gap-1">
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
                <fieldset disabled={!canEdit || busy} className="grid gap-3 sm:grid-cols-3">
                  <label className="space-y-1 text-xs text-muted-foreground">
                    <span>题号</span>
                    <Input
                      aria-label={`${entry.problemPublicId} 题号`}
                      maxLength={8}
                      value={entry.label}
                      onChange={(e) => change(index, 'label', e.target.value)}
                    />
                  </label>
                  <label className="space-y-1 text-xs text-muted-foreground">
                    <span>分值</span>
                    <Input
                      aria-label={`${entry.problemPublicId} 分值`}
                      type="number"
                      min={1}
                      max={100000}
                      value={entry.points}
                      onChange={(e) => change(index, 'points', Number(e.target.value))}
                    />
                  </label>
                  <label className="space-y-1 text-xs text-muted-foreground">
                    <span>颜色（可选）</span>
                    <Input
                      aria-label={`${entry.problemPublicId} 颜色`}
                      maxLength={32}
                      placeholder="#64748b"
                      value={entry.color}
                      onChange={(e) => change(index, 'color', e.target.value)}
                    />
                  </label>
                </fieldset>
              </li>
            ))}
          </ol>
        ) : (
          <p className="text-sm text-muted-foreground">尚未加入题目。</p>
        )}
        {canEdit && (
          <div className="flex flex-wrap items-center gap-3 border-t pt-4">
            <Button onClick={() => void save()} loading={busy} disabled={!dirty}>
              保存编排
            </Button>
            <Button
              variant="ghost"
              disabled={busy || !dirty}
              onClick={() => {
                setEntries(problems.map((p) => ({ ...p })))
                setDirty(false)
                setError(null)
              }}
            >
              撤销未保存修改
            </Button>
            <span className="text-xs text-muted-foreground">
              {dirty ? '有未保存修改' : `${entries.length} 道题目`}
            </span>
          </div>
        )}
      </Card>
      {canEdit && (
        <Card className="space-y-4 p-4 sm:p-5">
          <h2 className="font-medium">加入本域题目</h2>
          <p className="text-xs text-muted-foreground">
            只可加入有权访问且已发布的本域题目。外域材料请先在出题工作台复制。
          </p>
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
              placeholder="题目名称或来源"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <Button type="submit" variant="outline">
              <Search />
              搜索
            </Button>
          </form>
          {choices.loading ? (
            <p role="status">正在查找题目…</p>
          ) : choices.error ? (
            <div className="flex flex-wrap items-center gap-2">
              <p role="alert" className="text-sm text-destructive">
                {choices.error}
              </p>
              <Button variant="outline" onClick={choices.reload}>
                重试
              </Button>
            </div>
          ) : (
            <>
              {choices.data?.items.length ? (
                <ul className="divide-y">
                  {choices.data.items.map((problem) => (
                    <li key={problem.id} className="flex items-center gap-3 py-3 text-sm">
                      <div className="min-w-0 flex-1">
                        <span className="mr-2 font-mono text-muted-foreground">
                          {problem.publicId}
                        </span>
                        {problem.title}
                        <p className="text-xs text-muted-foreground">
                          {problem.publishedVersion
                            ? `当前发布 v${problem.publishedVersion}`
                            : '尚未发布，不能加入比赛'}
                        </p>
                      </div>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={
                          busy ||
                          !problem.publishedVersion ||
                          entries.some((p) => p.problemId === problem.id)
                        }
                        onClick={() => add(problem)}
                      >
                        <Plus />
                        {entries.some((p) => p.problemId === problem.id) ? '已加入' : '加入'}
                      </Button>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-muted-foreground">没有匹配的可见题目。</p>
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
  )
}
