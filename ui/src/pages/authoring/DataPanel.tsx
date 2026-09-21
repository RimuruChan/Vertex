import { useSearchParams } from 'react-router-dom'
import GenerationPanel from './GenerationPanel'
import { useEffect, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Plus, Upload, Files, CheckCircle2, AlertCircle, Wand2, ChevronDown } from 'lucide-react'
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
import { pairTestFiles, pairDraft, saveTestDrafts } from './data-actions'

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
}) {
  const [params] = useSearchParams()
  const requestedTab = params.get('data')
  const confirm = useConfirm()
  const api = useDomainAPI(),
    fileInput = useRef<HTMLInputElement>(null)
  const [tab, setTab] = useState(requestedTab === 'generation' ? 'generation' : 'test'),
    [dialog, setDialog] = useState<'upload' | 'manual' | null>(null),
    [files, setFiles] = useState<File[]>([])
  const [name, setName] = useState(''),
    [input, setInput] = useState(''),
    [answer, setAnswer] = useState(''),
    [sample, setSample] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [done, setDone] = useState(0)
  const [added, setAdded] = useState<{ count: number; entry?: DomainTreeEntry }>()
  useEffect(() => {
    if (requestedTab === 'generation') setTab('generation')
  }, [requestedTab])
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
  let pairs: ReturnType<typeof pairTestFiles> = [],
    pairError = ''
  try {
    pairs = pairTestFiles(files)
  } catch (e) {
    pairError = (e as Error).message
  }
  const incomplete = pairs.some((pair) => pair.error || !pair.input || !pair.answer)
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
              <DropdownMenuItem onSelect={() => setTab('generation')}>
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
              disabled={disabled}
              onClick={() => setTab(id)}
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
          <span>已添加 {added.count} 个测试点</span>
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
                  }}
                >
                  <Files className="mb-1 size-6 text-primary" />
                  <span className="font-medium">拖入数据文件，或点击选择</span>
                  <span className="text-xs text-muted-foreground">
                    如 1.in + 1.out · 每次最多 100 组
                  </span>
                </button>
                {!!files.length && (
                  <div className="max-h-64 overflow-auto rounded-xl border">
                    <table className="w-full text-left text-sm">
                      <thead className="bg-muted/40 text-xs text-muted-foreground">
                        <tr>
                          <th className="p-3">测试点</th>
                          <th className="p-3">输入</th>
                          <th className="p-3">答案</th>
                          <th className="p-3">状态</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pairs.map((pair) => (
                          <tr key={pair.name} className="border-t">
                            <td className="p-3 font-medium">{pair.name}</td>
                            <td className="p-3 text-xs">
                              {pair.input ? formatFileSize(pair.input.size) : '缺少输入'}
                            </td>
                            <td className="p-3 text-xs">
                              {pair.answer ? formatFileSize(pair.answer.size) : '缺少答案'}
                            </td>
                            <td className="p-3">
                              {pair.error || !pair.input || !pair.answer ? (
                                <span className="text-xs text-amber-600">
                                  {pair.error || '未配对'}
                                </span>
                              ) : (
                                <span className="inline-flex items-center gap-1 text-xs text-primary">
                                  <CheckCircle2 className="size-4" />
                                  已配对
                                </span>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                {!!files.length && (
                  <Button size="sm" variant="ghost" onClick={() => setFiles([])}>
                    清空所选文件
                  </Button>
                )}
              </>
            )}
            {(error || pairError) && (
              <p role="alert" className="flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                {error || pairError}
              </p>
            )}
          </fieldset>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <span className="text-xs text-muted-foreground">
              {busy
                ? `正在保存 ${done} / ${dialog === 'manual' ? 1 : pairs.length}`
                : sample
                  ? '样例随题目发布后展示给参赛者。'
                  : '评测数据不会公开给参赛者。'}
            </span>
            <Button
              loading={busy}
              disabled={
                busy ||
                !!pairError ||
                (dialog === 'upload' && (!pairs.length || incomplete || pairs.length > 100))
              }
              onClick={() => void apply()}
            >
              添加{dialog === 'upload' ? ` ${pairs.length} 个测试点` : '测试点'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
