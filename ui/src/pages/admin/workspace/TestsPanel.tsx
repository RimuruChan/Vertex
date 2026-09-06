import { useState } from 'react'
import { ArrowDown, ArrowUp, Pencil, Plus, Trash2 } from 'lucide-react'
import {
  deleteApiAdminProblemsIdTestsTestId as deleteTest,
  postApiAdminProblemsIdTests as createTest,
  postApiAdminProblemsIdTestsTestIdMove as moveTest,
  putApiAdminProblemsIdTestsTestId as updateTest,
} from '@/generated/api/vertex'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useConfirm } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState } from '@/components/ui/misc'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'
import type { PackageFile, PackageTest } from './types'

type Draft = {
  id?: number
  source: 'manual' | 'generator'
  inputData: string
  generateCmd: string
  group: string
  points: number
  isSample: boolean
  description: string
}

const emptyDraft: Draft = {
  source: 'manual',
  inputData: '',
  generateCmd: '',
  group: '',
  points: 0,
  isSample: false,
  description: '',
}

function toDraft(test: PackageTest): Draft {
  return {
    id: test.id,
    source: test.source === 'generator' ? 'generator' : 'manual',
    inputData: test.inputData,
    generateCmd: test.generateCmd,
    group: test.group,
    points: test.points,
    isSample: test.isSample,
    description: test.description,
  }
}

/**
 * The test plan. Answers never appear here: they are produced by the model
 * solution during a build, which is what keeps inputs and answers in sync.
 */
export default function TestsPanel({
  problemId,
  tests,
  files,
  onChanged,
}: {
  problemId: string
  tests: PackageTest[]
  files: PackageFile[]
  onChanged: () => void
}) {
  const toast = useToast()
  const confirm = useConfirm()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  const generators = files.filter((file) => file.kind === 'generator')

  async function handleSave() {
    if (!draft) return
    setSaving(true)
    try {
      const payload = {
        source: draft.source,
        inputData: draft.source === 'manual' ? draft.inputData : undefined,
        generateCmd: draft.source === 'generator' ? draft.generateCmd : undefined,
        group: draft.group || undefined,
        points: draft.points || undefined,
        isSample: draft.isSample || undefined,
        description: draft.description || undefined,
      }
      if (draft.id) {
        await updateTest(problemId, draft.id, payload)
      } else {
        await createTest(problemId, payload)
      }
      toast.success('测试点已保存')
      setDraft(null)
      onChanged()
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(test: PackageTest) {
    if (deletingId !== null) return
    const accepted = await confirm({
      title: `删除测试点 ${test.index}？`,
      description: `${test.isSample ? '它也会从下一次构建生成的公开题面样例中消失。' : ''}后续测试点会自动重新编号，删除无法撤销；当前已发布的题目包在重新构建前不受影响。`,
      confirmLabel: '删除测试点',
      destructive: true,
    })
    if (!accepted) return
    setDeletingId(test.id)
    try {
      await deleteTest(problemId, test.id)
      toast.success('已删除,后续测试点已顺延编号')
      onChanged()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setDeletingId(null)
    }
  }

  async function handleMove(test: PackageTest, delta: number) {
    const position = test.index + delta
    if (position < 1 || position > tests.length) return
    setBusy(true)
    try {
      await moveTest(problemId, test.id, { position })
      onChanged()
    } catch (error) {
      toast.error(apiError(error, '调整顺序失败'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card className="overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border p-4">
        <div>
          <p className="text-sm font-medium">测试点({tests.length})</p>
          <p className="text-xs text-muted-foreground">
            答案由标程在构建时生成;这里只定义输入。样例测试点会渲染进公开题面。
          </p>
        </div>
        <Button size="sm" onClick={() => setDraft(emptyDraft)}>
          <Plus />
          添加测试点
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="w-14">#</TableHead>
            <TableHead className="w-24">来源</TableHead>
            <TableHead>输入 / 生成命令</TableHead>
            <TableHead className="hidden w-28 lg:table-cell">分组</TableHead>
            <TableHead className="w-16 text-right">分值</TableHead>
            <TableHead className="w-20">样例</TableHead>
            <TableHead className="w-36 text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {tests.length === 0 ? (
            <TableEmpty colSpan={7}>
              <EmptyState
                title="还没有测试点"
                description="可以直接粘贴一组手工数据,或用生成器命令批量产生。"
                action={<Button onClick={() => setDraft(emptyDraft)}>添加测试点</Button>}
              />
            </TableEmpty>
          ) : (
            tests.map((test) => (
              <TableRow key={test.id}>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {test.index}
                </TableCell>
                <TableCell>
                  <Badge variant={test.source === 'generator' ? 'secondary' : 'outline'}>
                    {test.source === 'generator' ? '生成器' : '手工'}
                  </Badge>
                </TableCell>
                <TableCell className="max-w-0">
                  <div className="truncate font-mono text-xs">
                    {test.source === 'generator'
                      ? test.generateCmd
                      : test.inputData.split('\n')[0] || '(空)'}
                  </div>
                  {test.description ? (
                    <div className="truncate text-xs text-muted-foreground">{test.description}</div>
                  ) : null}
                </TableCell>
                <TableCell className="hidden text-muted-foreground lg:table-cell">
                  {test.group || '—'}
                </TableCell>
                <TableCell className="text-right tabular-nums">{test.points || '—'}</TableCell>
                <TableCell>
                  {test.isSample ? <Badge variant="success">样例</Badge> : null}
                </TableCell>
                <TableCell>
                  <div className="flex items-center justify-end gap-1">
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="上移"
                      disabled={busy || test.index === 1}
                      onClick={() => handleMove(test, -1)}
                    >
                      <ArrowUp />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="下移"
                      disabled={busy || test.index === tests.length}
                      onClick={() => handleMove(test, 1)}
                    >
                      <ArrowDown />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="编辑"
                      onClick={() => setDraft(toDraft(test))}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="删除"
                      className="hover:text-destructive"
                      disabled={deletingId !== null}
                      loading={deletingId === test.id}
                      onClick={() => handleDelete(test)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      <Dialog open={draft !== null} onOpenChange={(open) => !open && setDraft(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{draft?.id ? '编辑测试点' : '添加测试点'}</DialogTitle>
          </DialogHeader>
          {draft ? (
            <div className="flex flex-col gap-3">
              <div className="grid gap-3 sm:grid-cols-3">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="test-source">来源</Label>
                  <Select
                    value={draft.source}
                    onValueChange={(value) =>
                      setDraft({ ...draft, source: value as 'manual' | 'generator' })
                    }
                  >
                    <SelectTrigger id="test-source">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="manual">手工输入</SelectItem>
                      <SelectItem value="generator">生成器命令</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="test-group">分组</Label>
                  <Input
                    id="test-group"
                    value={draft.group}
                    onChange={(event) => setDraft({ ...draft, group: event.target.value })}
                    placeholder="可留空"
                  />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="test-points">分值</Label>
                  <Input
                    id="test-points"
                    type="number"
                    min={0}
                    max={1000}
                    value={draft.points}
                    onChange={(event) =>
                      setDraft({ ...draft, points: Number(event.target.value) || 0 })
                    }
                  />
                </div>
              </div>

              {draft.source === 'manual' ? (
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="test-input">输入数据</Label>
                  <Textarea
                    id="test-input"
                    rows={10}
                    className="font-mono text-xs"
                    value={draft.inputData}
                    onChange={(event) => setDraft({ ...draft, inputData: event.target.value })}
                    placeholder={'3\n1 2 3'}
                  />
                </div>
              ) : (
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="test-command">生成命令</Label>
                  <Input
                    id="test-command"
                    className="font-mono"
                    value={draft.generateCmd}
                    onChange={(event) => setDraft({ ...draft, generateCmd: event.target.value })}
                    placeholder="gen 100000 1000000000"
                  />
                  <p className="text-xs text-muted-foreground">
                    第一个词是生成器名称,其余作为 argv 原样传入(不经过 shell)。
                    {generators.length > 0
                      ? ` 可用生成器:${generators.map((item) => item.name).join('、')}`
                      : ' 还没有生成器,请先在「文件」中添加。'}
                  </p>
                </div>
              )}

              <div className="flex flex-col gap-1.5">
                <Label htmlFor="test-description">备注</Label>
                <Input
                  id="test-description"
                  value={draft.description}
                  onChange={(event) => setDraft({ ...draft, description: event.target.value })}
                  placeholder="例如:极大数据、全相同元素"
                />
              </div>

              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="size-4 accent-primary"
                  checked={draft.isSample}
                  onChange={(event) => setDraft({ ...draft, isSample: event.target.checked })}
                />
                作为公开题面中的样例
              </label>
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDraft(null)}>
              取消
            </Button>
            <Button loading={saving} onClick={handleSave}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
