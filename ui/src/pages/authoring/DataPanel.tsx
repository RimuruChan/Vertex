import { useSearchParams } from 'react-router-dom'
import GenerationPanel from './GenerationPanel'
import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import {
  Plus,
  Upload,
  Files,
  CheckCircle2,
  AlertCircle,
  Wand2,
  ChevronDown,
  X,
  Trash2,
} from 'lucide-react'
import type { DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { apiError, formatFileSize } from '@/lib/format'
import { Field } from './FormFields'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import MaterialBrowser from './MaterialBrowser'
import { pairDraft, saveTestDrafts } from './data-actions'
import { reviewTestFiles } from './data-review'

export default function DataPanel({
  problemId,
  copy,
  revision,
  canEdit,
  disabled,
  onSaved,
  onDirty,
  onBusy,
  onSelect,
  onAdvancedCreate,
  requestedTab,
}: {
  problemId: string
  copy: DomainWorkingCopy
  revision?: number
  canEdit: boolean
  disabled: boolean
  onDirty: (dirty: boolean) => void
  onSaved: (copy: DomainWorkingCopy) => void
  onBusy: (busy: boolean) => void
  onSelect: (entry: DomainTreeEntry) => void
  onAdvancedCreate: (kind: string) => void
  requestedTab?: 'test' | 'group' | 'validation' | 'generation'
}) {
  const [params, setParams] = useSearchParams()
  const requested = requestedTab ?? params.get('data')
  const initialTab = ['test', 'group', 'validation', 'generation'].includes(requested ?? '')
    ? requested!
    : 'test'
  const confirm = useConfirm()
  const api = useDomainAPI(),
    fileInput = useRef<HTMLInputElement>(null)
  const [tab, setTab] = useState(initialTab),
    [dialog, setDialog] = useState<'upload' | 'manual' | null>(null),
    [files, setFiles] = useState<File[]>([])
  const [name, setName] = useState(''),
    [input, setInput] = useState(''),
    [answer, setAnswer] = useState(''),
    [sample, setSample] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [done, setDone] = useState(0)
  const [added, setAdded] = useState<{ count: number; sample: boolean; entry?: DomainTreeEntry }>()
  useEffect(() => {
    setTab(initialTab)
  }, [initialTab])
  function chooseTab(value: string) {
    if (disabled || busy) return
    setTab(value)
    setParams(
      (current) => {
        current.set('data', value)
        current.delete('entry')
        if (value !== 'generation') current.delete('plan')
        return current
      },
      { replace: true },
    )
  }
  const hasDraft = dialog !== null && (files.length > 0 || !!name || !!input || !!answer)
  useEffect(() => {
    onDirty(hasDraft)
    return () => onDirty(false)
  }, [hasDraft, onDirty])
  async function close() {
    if (busy) return
    if (
      hasDraft &&
      !(await confirm({
        title: '放弃这次添加？',
        description: '尚未添加的数据和输入内容会被清空。',
        confirmLabel: '放弃添加',
        destructive: true,
      }))
    )
      return
    setDialog(null)
  }
  const { pairs, rejected, readyCount } = reviewTestFiles(files)
  const incomplete = readyCount !== pairs.length || rejected.length > 0
  function removeFiles(indexes: number[]) {
    setFiles((current) => current.filter((_, index) => !indexes.includes(index)))
    setError('')
  }
  function open(mode: 'upload' | 'manual') {
    setDialog(mode)
    setFiles([])
    setName('')
    setInput('')
    setAnswer('')
    setSample(false)
    setError('')
  }
  async function apply() {
    if (
      busy ||
      !canEdit ||
      (dialog === 'upload' && (!pairs.length || incomplete || pairs.length > 100))
    )
      return
    setBusy(true)
    onBusy(true)
    setError('')
    setDone(0)
    try {
      const selected =
        dialog === 'manual'
          ? [
              {
                name:
                  name.trim() ||
                  `测试 ${copy.tree.entries.filter((e) => e.kind === 'test').length + 1}`,
                input: new File([input], 'input.in'),
                answer: new File([answer], 'answer.ans'),
              },
            ]
          : pairs
      const next = await saveTestDrafts(
        api,
        problemId,
        copy,
        selected.map((p) => pairDraft(p, sample)),
        setDone,
      )
      onSaved(next)
      setAdded({
        count: selected.length,
        sample,
        entry: next.tree.entries.find(
          (entry) => entry.kind === 'test' && !copy.tree.entries.some((old) => old.id === entry.id),
        ),
      })
      setDialog(null)
    } catch (e) {
      setError(apiError(e, '数据没有保存，请重试'))
    } finally {
      setBusy(false)
      onBusy(false)
    }
  }
  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">测试数据</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {copy.tree.entries.filter((e) => e.kind === 'test').length} 个测试点 ·{' '}
            {copy.tree.entries.filter((e) => e.kind === 'generation').length} 个生成方案
          </p>
        </div>
        {canEdit && tab === 'test' && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button disabled={disabled}>
                <Plus />
                添加测试
                <ChevronDown />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => open('manual')}>
                <Plus />
                手动编写输入与答案
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => open('upload')}>
                <Upload />
                导入成对数据
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => chooseTab('generation')}>
                <Wand2 />
                用生成器批量构造
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </header>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3">
        <div className="flex flex-wrap gap-1">
          {[
            ['test', '测试点'],
            ['generation', '批量生成'],
            ['group', '测试组'],
            ['validation', '校验器自测'],
          ].map(([id, label]) => (
            <Button
              key={id}
              size="sm"
              variant={tab === id ? 'secondary' : 'ghost'}
              aria-pressed={tab === id}
              disabled={disabled}
              onClick={() => chooseTab(id)}
            >
              {label}
            </Button>
          ))}
        </div>
        {canEdit && ['group', 'validation'].includes(tab) && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onAdvancedCreate(tab)}
            disabled={disabled}
          >
            <Plus />
            添加{tab === 'group' ? '测试组' : '自测'}
          </Button>
        )}
      </div>
      {added && (
        <div
          role="status"
          className="flex items-center justify-between gap-3 rounded-lg bg-primary/5 px-4 py-3 text-sm"
        >
          <div className="flex min-w-0 items-start gap-2">
            <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-primary" />
            <div>
              <p>
                已添加 {added.count} 个{added.sample ? '公开样例' : '评测测试点'}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                已保存到个人草稿，可继续添加或检查数据。
              </p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {added.entry && (
              <Button
                size="sm"
                variant="ghost"
                disabled={disabled}
                onClick={() => onSelect(added.entry!)}
              >
                查看新增测试
              </Button>
            )}
            <Button
              size="icon"
              variant="ghost"
              className="size-8"
              aria-label="关闭添加成功提示"
              onClick={() => setAdded(undefined)}
            >
              <X />
            </Button>
          </div>
        </div>
      )}
      {tab === 'generation' ? (
        <GenerationPanel
          initialPlanId={params.get('plan') ?? undefined}
          problemId={problemId}
          copy={copy}
          revision={revision}
          canEdit={canEdit}
          onSaved={onSaved}
          onBusy={onBusy}
          onDirty={onDirty}
        />
      ) : (
        <MaterialBrowser
          key={tab}
          activeKind={tab}
          hideKindTabs
          problemId={problemId}
          mode="tests"
          copy={copy}
          revision={revision}
          canEdit={canEdit}
          disabled={disabled}
          onSelect={onSelect}
          onSaved={onSaved}
          onBusy={onBusy}
          onCreate={tab === 'test' ? () => open('upload') : undefined}
        />
      )}
      <Dialog
        open={dialog !== null}
        onOpenChange={(value) => {
          if (!value) void close()
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogTitle>{dialog === 'upload' ? '上传测试数据' : '添加测试点'}</DialogTitle>
          <DialogDescription>
            {dialog === 'upload'
              ? '选择 .in 与同名 .out / .ans 文件。确认配对后一起加入题目。'
              : '直接填写输入与答案，也可以稍后重新上传文件。'}
          </DialogDescription>
          <fieldset disabled={busy} className="min-w-0 space-y-5">
            <div className="flex items-center gap-2 text-sm">
              <span className="mr-2 text-muted-foreground">用途</span>
              {[
                [false, '评测数据'],
                [true, '公开样例'],
              ].map(([value, label]) => (
                <Button
                  key={String(value)}
                  size="sm"
                  variant={sample === value ? 'secondary' : 'outline'}
                  aria-pressed={sample === value}
                  onClick={() => setSample(value as boolean)}
                >
                  {label as string}
                </Button>
              ))}
            </div>
            {dialog === 'manual' ? (
              <>
                <Field label="测试点名称">
                  <Input
                    aria-label="测试点名称"
                    placeholder="例如：最小规模、全相等、最大数据"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </Field>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="输入">
                    <Textarea
                      aria-label="测试输入"
                      className="h-52 resize-none font-mono"
                      value={input}
                      onChange={(e) => setInput(e.target.value)}
                      spellCheck={false}
                    />
                  </Field>
                  <Field label="期望输出">
                    <Textarea
                      aria-label="测试答案"
                      className="h-52 resize-none font-mono"
                      value={answer}
                      onChange={(e) => setAnswer(e.target.value)}
                      spellCheck={false}
                    />
                  </Field>
                </div>
              </>
            ) : (
              <>
                <input
                  ref={fileInput}
                  type="file"
                  multiple
                  accept=".in,.out,.ans"
                  className="hidden"
                  aria-label="选择输入和答案文件"
                  onChange={(e) => {
                    setFiles((current) => [...current, ...Array.from(e.target.files ?? [])])
                    setError('')
                    e.target.value = ''
                  }}
                />
                <button
                  type="button"
                  className="flex w-full flex-col items-center gap-2 rounded-xl border-2 border-dashed bg-muted/20 px-6 py-7 text-sm transition-colors hover:border-primary/50 hover:bg-primary/5"
                  onClick={() => fileInput.current?.click()}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    e.preventDefault()
                    if (busy) return
                    setFiles((current) => [...current, ...Array.from(e.dataTransfer.files)])
                    setError('')
                  }}
                >
                  <Files className="mb-1 size-6 text-primary" />
                  <span className="font-medium">
                    {files.length
                      ? '继续添加文件，补齐缺失的输入或答案'
                      : '拖入数据文件，或点击选择'}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    如 1.in + 1.out · 每次最多 100 组 · 单文件不超过 64 MiB
                  </span>
                </button>
                {!!files.length && (
                  <div className="space-y-3">
                    <div
                      className="flex flex-wrap items-center justify-between gap-2 text-sm"
                      role="status"
                    >
                      <span>
                        {files.length} 个文件 ·{' '}
                        <span className="font-medium text-primary">{readyCount} 组已就绪</span>
                        {pairs.length > readyCount &&
                          ` · ${pairs.length - readyCount} 组待补齐或修正`}
                        {rejected.length > 0 && ` · ${rejected.length} 个无效文件`}
                      </span>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setFiles([])
                          setError('')
                        }}
                      >
                        清空全部
                      </Button>
                    </div>
                    {pairs.length > 100 && (
                      <p role="alert" className="text-sm text-destructive">
                        每次最多添加 100 组，请移除多余组后分批添加。
                      </p>
                    )}
                    {!!rejected.length && (
                      <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 space-y-2">
                        {rejected.map(({ file, index, error }) => (
                          <div key={index} className="flex items-start justify-between gap-2">
                            <p className="min-w-0 break-all text-xs text-destructive">{error}</p>
                            <Button
                              size="icon"
                              variant="ghost"
                              className="size-7 shrink-0"
                              aria-label={`移除无效文件 ${file.name}`}
                              onClick={() => removeFiles([index])}
                            >
                              <X />
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}
                    {!!pairs.length && (
                      <div className="max-h-72 overflow-auto rounded-xl border">
                        <table className="w-full text-left text-sm">
                          <thead className="sticky top-0 bg-card text-xs text-muted-foreground">
                            <tr>
                              <th className="p-3">测试点</th>
                              <th className="p-3">输入</th>
                              <th className="p-3">答案</th>
                              <th className="p-3">状态</th>
                              <th className="p-3">
                                <span className="sr-only">移除</span>
                              </th>
                            </tr>
                          </thead>
                          <tbody>
                            {pairs.map((pair) => (
                              <tr key={pair.name} className="border-t">
                                <td className="p-3 font-medium max-w-36 break-all">{pair.name}</td>
                                {(['input', 'answer'] as const).map((part) => (
                                  <td key={part} className="p-3 text-xs">
                                    {pair.files.filter((file) => file.part === part).length ? (
                                      <div className="space-y-1">
                                        {pair.files
                                          .filter((file) => file.part === part)
                                          .map(({ file, index }) => (
                                            <div
                                              key={index}
                                              className="flex items-center gap-1 rounded-md bg-muted/40 pl-2"
                                            >
                                              <span className="min-w-0 flex-1">
                                                <span
                                                  className="block max-w-40 truncate"
                                                  title={file.name}
                                                >
                                                  {file.name}
                                                </span>
                                                <span className="text-muted-foreground">
                                                  {formatFileSize(file.size)}
                                                </span>
                                              </span>
                                              <Button
                                                size="icon"
                                                variant="ghost"
                                                className="size-7 shrink-0"
                                                aria-label={`移除文件 ${file.name}`}
                                                onClick={() => removeFiles([index])}
                                              >
                                                <X />
                                              </Button>
                                            </div>
                                          ))}
                                      </div>
                                    ) : (
                                      <span className="text-verdict-tle">
                                        {part === 'input' ? '等待输入文件' : '等待答案文件'}
                                      </span>
                                    )}
                                  </td>
                                ))}
                                <td className="p-3">
                                  {pair.issues.length ? (
                                    <span className="text-xs text-verdict-tle">
                                      {pair.issues.join('；')}
                                    </span>
                                  ) : (
                                    <span className="inline-flex items-center gap-1 text-xs text-primary">
                                      <CheckCircle2 className="size-4" />
                                      已配对
                                    </span>
                                  )}
                                </td>
                                <td className="p-2">
                                  <Button
                                    size="icon"
                                    variant="ghost"
                                    className="size-8"
                                    aria-label={`移除整组 ${pair.name}`}
                                    title="移除整组"
                                    onClick={() =>
                                      removeFiles(pair.files.map(({ index }) => index))
                                    }
                                  >
                                    <Trash2 />
                                  </Button>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                    <p className="text-xs text-muted-foreground">
                      可移除重复文件或整组数据，再继续添加文件。所有组完整配对后才会写入草稿。
                    </p>
                  </div>
                )}
              </>
            )}
            {error && (
              <p role="alert" className="flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                {error}
              </p>
            )}
          </fieldset>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <span className="text-xs text-muted-foreground">
              {busy
                ? done === (dialog === 'manual' ? 1 : pairs.length)
                  ? '文件已就绪，正在写入个人草稿…'
                  : `正在上传与处理 ${done} / ${dialog === 'manual' ? 1 : pairs.length} 个测试点…`
                : sample
                  ? '样例随题目发布后展示给参赛者。'
                  : '评测数据不会公开给参赛者。'}
            </span>
            <div className="flex items-center gap-2">
              <Button variant="outline" disabled={busy} onClick={() => void close()}>
                取消
              </Button>
              <Button
                loading={busy}
                disabled={
                  busy ||
                  !canEdit ||
                  (dialog === 'upload' && (!pairs.length || incomplete || pairs.length > 100))
                }
                onClick={() => void apply()}
              >
                添加{dialog === 'upload' ? ` ${pairs.length} 个测试点` : '测试点'}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
