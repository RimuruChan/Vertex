import { useEffect, useState } from 'react'
import { Play, Plus, Trash2, Eye, Wand2, Save, ArrowLeft, ChevronRight } from 'lucide-react'
import type {
  DomainGenerationPlan,
  DomainGeneratedCase,
  DomainMaterialView,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useNavigate, Link } from '@/domain/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Choice, Field } from './FormFields'
import { apiError } from '@/lib/format'
import { useConfirm } from '@/components/ui/confirm-dialog'

const fresh = (): DomainGenerationPlan => ({
  schemaVersion: 1,
  name: '随机数据覆盖',
  generator: '',
  solution: '',
  group: '',
  rules: [
    {
      id: crypto.randomUUID(),
      name: '随机测试',
      count: 10,
      seedStart: 1,
      parameters: '100 {seed}',
    },
  ],
})
export default function GenerationPanel({
  initialPlanId,
  problemId,
  copy,
  revision,
  canEdit,
  onSaved,
  onBusy,
  onDirty,
}: {
  initialPlanId?: string
  problemId: string
  copy: DomainWorkingCopy
  revision?: number
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onBusy: (busy: boolean) => void
  onDirty: (dirty: boolean) => void
}) {
  const api = useDomainAPI(),
    navigate = useNavigate(),
    confirm = useConfirm()
  const [materials, setMaterials] = useState<DomainMaterialView[]>([]),
    [id, setID] = useState(''),
    [plan, setPlan] = useState<DomainGenerationPlan>(fresh),
    [baseline, setBaseline] = useState('')
  const [editing, setEditing] = useState(!!initialPlanId),
    [step, setStep] = useState(0)
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [preview, setPreview] = useState<DomainGeneratedCase[]>(),
    [previewKey, setPreviewKey] = useState('')
  const signature = JSON.stringify(plan),
    dirty = baseline !== '' && signature !== baseline
  useEffect(() => {
    onBusy(busy || loading)
    return () => onBusy(false)
  }, [busy, loading, onBusy])
  useEffect(() => {
    onDirty(dirty)
    return () => onDirty(false)
  }, [dirty, onDirty])
  useEffect(() => {
    let live = true
    setLoading(true)
    void (async () => {
      const all: DomainMaterialView[] = []
      await Promise.all(
        ['program', 'generation', 'group'].map(async (kind) => {
          let after: string | undefined
          do {
            const page = await api.getApiAuthoringProblemsIdMaterials(problemId, {
              kind,
              limit: 100,
              after,
              ...(revision ? { revision } : { etag: copy.etag }),
            })
            all.push(...page.items)
            after = page.next || undefined
          } while (after)
        }),
      )
      if (!live) return
      setMaterials(all)
      if (!id) {
        const first =
          all.find((m) => m.generation && m.entry.id === initialPlanId) ??
          all.find((m) => m.generation)
        if (first) {
          setID(first.entry.id)
          setPlan(first.generation!)
          setBaseline(JSON.stringify(first.generation))
        } else {
          const p = fresh()
          p.generator = all.find((m) => m.program?.role === 'generator')?.entry.id ?? ''
          p.solution =
            all.find(
              (m) =>
                m.program?.role === 'solution' &&
                JSON.stringify(m.program.expectedVerdicts) === '["Accepted"]',
            )?.entry.id ?? ''
          setID(crypto.randomUUID())
          setPlan(p)
          setBaseline(JSON.stringify(p))
        }
      }
    })()
      .catch((e) => {
        if (live) setError(apiError(e, '生成方案加载失败'))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [api, problemId, copy.etag, revision])
  async function choose(item?: DomainMaterialView) {
    if (
      dirty &&
      !(await confirm({
        title: '放弃当前方案的未保存修改？',
        description: '已保存的生成方案和测试点不会改变。',
        confirmLabel: '切换方案',
        destructive: true,
      }))
    )
      return
    const p = item?.generation ?? fresh()
    if (!item) {
      p.generator = materials.find((m) => m.program?.role === 'generator')?.entry.id ?? ''
      p.solution =
        materials.find(
          (m) => m.program?.role === 'solution' && m.program.expectedVerdicts.includes('Accepted'),
        )?.entry.id ?? ''
    }
    setEditing(true)
    setStep(0)
    setID(item?.entry.id ?? crypto.randomUUID())
    setPlan(p)
    setBaseline(JSON.stringify(p))
    setPreview(undefined)
    setError('')
    setNotice('')
  }
  async function closeEditor() {
    if (busy) return
    if (
      dirty &&
      !(await confirm({
        title: '放弃方案的未保存修改？',
        description: '已保存的测试点不会改变。',
        confirmLabel: '放弃修改',
        destructive: true,
      }))
    )
      return
    if (baseline) setPlan(JSON.parse(baseline))
    setEditing(false)
    setError('')
  }
  async function inspect() {
    setBusy(true)
    setError('')
    try {
      const result = await api.postApiAuthoringProblemsIdGeneration(problemId, {
        etag: copy.etag,
        id,
        plan,
        preview: true,
      })
      setPreview(result.cases)
      setPreviewKey(signature + copy.etag)
      setStep(1)
    } catch (e) {
      setError(apiError(e, '方案不能展开'))
    } finally {
      setBusy(false)
    }
  }
  async function apply(run: boolean) {
    setBusy(true)
    setError('')
    setNotice('')
    let applied = false
    try {
      const result = await api.postApiAuthoringProblemsIdGeneration(problemId, {
        etag: copy.etag,
        id,
        plan,
        preview: false,
      })
      if (!result.copy) throw new Error('生成方案未保存')
      applied = true
      setBaseline(signature)
      onSaved(result.copy)
      setPreview(result.cases)
      setPreviewKey(signature + result.copy.etag)
      setNotice(
        `已更新 ${result.cases.length} 个测试点。输入与答案将在运行时由生成器和标准解产生。`,
      )
      if (!run) setEditing(false)
      if (run) {
        await api.postApiAuthoringProblemsIdChecks(problemId, { etag: result.copy.etag })
        navigate(`/authoring/${problemId}/checks`)
      }
    } catch (e) {
      setError(
        apiError(
          e,
          applied ? '方案已保存，但未能启动生成，请到检查页重试。' : '方案没有保存，请重试',
        ),
      )
    } finally {
      setBusy(false)
    }
  }
  const plans = materials.filter((m) => m.generation),
    total = plan.rules.reduce((sum, r) => sum + (Number.isFinite(r.count) ? r.count : 0), 0)
  const validPreview = preview && previewKey === signature + copy.etag
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="font-semibold">批量生成方案</h3>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            选择生成器，按参数和种子展开测试点，再用标准解生成答案。
          </p>
        </div>
        {canEdit && (
          <Button variant="outline" disabled={busy || loading} onClick={() => void choose()}>
            <Plus />
            新建方案
          </Button>
        )}
      </div>
      {loading ? (
        <p role="status" className="py-8 text-sm text-muted-foreground">
          正在读取生成方案…
        </p>
      ) : plans.length ? (
        <div className="divide-y overflow-hidden rounded-xl border bg-card">
          {plans.map((item) => {
            const value = item.generation!
            return (
              <button
                key={item.entry.id}
                type="button"
                disabled={busy}
                onClick={() => void choose(item)}
                className="flex w-full items-center gap-4 px-4 py-4 text-left transition-colors hover:bg-muted/30 sm:px-5"
              >
                <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
                  <Wand2 className="size-5" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{value.name}</span>
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {materials.find((m) => m.entry.id === value.generator)?.program?.name ??
                      '生成器未配置'}{' '}
                    · {value.rules.length} 条规则 · {value.rules.reduce((n, r) => n + r.count, 0)}{' '}
                    个测试点
                  </span>
                </span>
                <span className="hidden text-xs text-muted-foreground sm:inline">
                  {canEdit ? '编辑与生成' : '查看方案'}
                </span>
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
              </button>
            )
          })}
        </div>
      ) : (
        <div className="rounded-xl border border-dashed p-8 text-center">
          <Wand2 className="mx-auto mb-3 size-7 text-muted-foreground" />
          <p className="font-medium">用一组规则生成多份数据</p>
          <p className="mt-2 text-sm text-muted-foreground">
            选择生成器和标准解，预览每个测试点的参数，再确认生成。
          </p>
        </div>
      )}
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="rounded-lg border border-primary/20 bg-primary/5 p-3 text-sm">
          {notice}
        </p>
      )}
      <Dialog
        open={editing}
        onOpenChange={(open) => {
          if (!open) void closeEditor()
        }}
      >
        <DialogContent side="right" className="w-full max-w-[760px] gap-0 overflow-hidden p-0">
          <header className="shrink-0 border-b px-5 py-5 pr-16">
            <DialogTitle>
              {plans.some((p) => p.entry.id === id) ? '编辑生成方案' : '新建生成方案'}
            </DialogTitle>
            <DialogDescription className="mt-1">
              先配置规则，再核对展开的测试点。保存后统一运行生成器和标准解。
            </DialogDescription>
            <div className="mt-4 flex items-center gap-3 text-xs" aria-label="生成方案步骤">
              <span className={step === 0 ? 'font-medium text-primary' : 'text-muted-foreground'}>
                1 · 配置规则
              </span>
              <ChevronRight className="size-3" />
              <span className={step === 1 ? 'font-medium text-primary' : 'text-muted-foreground'}>
                2 · 核对测试点
              </span>
            </div>
          </header>
          <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-5">
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            {step === 0 && (
              <>
                <fieldset disabled={!canEdit || busy || loading} className="min-w-0 space-y-6">
                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field label="方案名称">
                      <Input
                        aria-label="生成方案名称"
                        value={plan.name}
                        onChange={(e) => setPlan({ ...plan, name: e.target.value })}
                      />
                    </Field>
                    <Choice
                      label="生成器"
                      value={plan.generator}
                      onChange={(generator) => setPlan({ ...plan, generator })}
                      options={[
                        ['', '请选择生成器'],
                        ...materials
                          .filter((m) => m.program?.role === 'generator')
                          .map((m) => [m.entry.id, m.program!.name] as [string, string]),
                      ]}
                    />
                    <Choice
                      label="答案来源"
                      value={plan.solution}
                      onChange={(solution) => setPlan({ ...plan, solution })}
                      options={[
                        ['', '请选择正确参考解'],
                        ...materials
                          .filter(
                            (m) =>
                              m.program?.role === 'solution' &&
                              JSON.stringify(m.program.expectedVerdicts) === '["Accepted"]',
                          )
                          .map((m) => [m.entry.id, m.program!.name] as [string, string]),
                      ]}
                    />
                    <Choice
                      label="测试组"
                      value={plan.group}
                      onChange={(group) => setPlan({ ...plan, group })}
                      options={[
                        ['', '不分组'],
                        ...materials
                          .filter((m) => m.group)
                          .map((m) => [m.entry.id, m.group!.name] as [string, string]),
                      ]}
                    />
                  </div>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <h4 className="text-sm font-medium">生成规则</h4>
                      <span className="text-xs text-muted-foreground">
                        共 {total} 个测试点 · 最多 500 个
                      </span>
                    </div>
                    {plan.rules.map((rule, index) => (
                      <div
                        key={rule.id}
                        className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_36px] items-end gap-3 rounded-lg border bg-background/40 p-3 sm:grid-cols-[minmax(0,1fr)_100px_120px_36px]"
                      >
                        <div className="col-span-3 min-w-0 sm:col-span-1">
                          <Field label="规则名称">
                            <Input
                              aria-label={`规则 ${index + 1} 名称`}
                              value={rule.name}
                              onChange={(e) =>
                                setPlan({
                                  ...plan,
                                  rules: plan.rules.map((r) =>
                                    r.id === rule.id ? { ...r, name: e.target.value } : r,
                                  ),
                                })
                              }
                            />
                          </Field>
                        </div>
                        <Field label="测试数量">
                          <Input
                            aria-label={`规则 ${index + 1} 数量`}
                            type="number"
                            min={1}
                            max={500}
                            value={rule.count}
                            onChange={(e) =>
                              setPlan({
                                ...plan,
                                rules: plan.rules.map((r) =>
                                  r.id === rule.id ? { ...r, count: Number(e.target.value) } : r,
                                ),
                              })
                            }
                          />
                        </Field>
                        <Field label="起始种子">
                          <Input
                            aria-label={`规则 ${index + 1} 种子`}
                            type="number"
                            min={0}
                            value={rule.seedStart}
                            onChange={(e) =>
                              setPlan({
                                ...plan,
                                rules: plan.rules.map((r) =>
                                  r.id === rule.id
                                    ? { ...r, seedStart: Number(e.target.value) }
                                    : r,
                                ),
                              })
                            }
                          />
                        </Field>
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={`删除规则 ${index + 1}`}
                          disabled={plan.rules.length === 1}
                          onClick={() =>
                            setPlan({ ...plan, rules: plan.rules.filter((r) => r.id !== rule.id) })
                          }
                        >
                          <Trash2 />
                        </Button>
                        <div className="col-span-3 min-w-0 sm:col-span-4">
                          <Field label="生成器参数">
                            <Input
                              className="font-mono text-sm"
                              aria-label={`规则 ${index + 1} 参数`}
                              value={rule.parameters}
                              placeholder="100 1000 {seed}"
                              onChange={(e) =>
                                setPlan({
                                  ...plan,
                                  rules: plan.rules.map((r) =>
                                    r.id === rule.id ? { ...r, parameters: e.target.value } : r,
                                  ),
                                })
                              }
                            />
                          </Field>
                        </div>
                      </div>
                    ))}
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <p className="text-xs leading-5 text-muted-foreground">
                        {'{seed}'} 按起始种子递增；{'{index}'}{' '}
                        为规则内序号。带空格的参数用引号包裹。
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={plan.rules.length >= 50}
                        onClick={() =>
                          setPlan({
                            ...plan,
                            rules: [
                              ...plan.rules,
                              {
                                id: crypto.randomUUID(),
                                name: `规则 ${plan.rules.length + 1}`,
                                count: 10,
                                seedStart: 1,
                                parameters: '100 {seed}',
                              },
                            ],
                          })
                        }
                      >
                        <Plus />
                        添加规则
                      </Button>
                    </div>
                  </div>
                </fieldset>
                {!materials.some((m) => m.program?.role === 'generator') && !loading && (
                  <p className="text-sm text-muted-foreground">
                    还没有生成器。
                    <Link
                      className="ml-1 text-primary underline-offset-4 hover:underline"
                      to={`/authoring/${problemId}/programs`}
                    >
                      去编写生成器
                    </Link>
                  </p>
                )}
              </>
            )}
            {step === 1 && preview && (
              <section className="overflow-hidden rounded-xl border">
                <div className="flex items-center gap-2 border-b bg-muted/20 px-4 py-3 text-sm">
                  <Wand2 className="size-4" />
                  {validPreview ? '展开预览' : '方案已修改，请重新预览'} · {preview.length} 个测试点
                </div>
                <div className="overflow-auto">
                  <table className="w-full text-left text-xs">
                    <thead className="sticky top-0 bg-card text-muted-foreground">
                      <tr>
                        <th className="px-4 py-2">测试点</th>
                        <th className="px-4 py-2">种子</th>
                        <th className="px-4 py-2">实际参数</th>
                      </tr>
                    </thead>
                    <tbody>
                      {preview.map((item) => (
                        <tr key={item.id} className="border-t">
                          <td className="whitespace-nowrap px-4 py-2">{item.name}</td>
                          <td className="px-4 py-2 tabular-nums">{item.seed}</td>
                          <td className="px-4 py-2 font-mono">
                            {item.arguments.map((a) => JSON.stringify(a)).join(' ')}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            )}
          </div>
          <footer className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t bg-card px-5 py-4">
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => (step === 1 ? setStep(0) : void closeEditor())}
            >
              <ArrowLeft />
              {step === 1 ? '返回修改' : '取消'}
            </Button>
            {canEdit &&
              (step === 0 ? (
                <Button
                  loading={busy}
                  disabled={loading || !plan.generator || !plan.solution}
                  onClick={() => void inspect()}
                >
                  <Eye />
                  预览 {total} 个测试点
                  <ChevronRight />
                </Button>
              ) : (
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    disabled={busy || !validPreview}
                    onClick={() => void apply(false)}
                  >
                    <Save />
                    仅保存方案
                  </Button>
                  <Button loading={busy} disabled={!validPreview} onClick={() => void apply(true)}>
                    <Play />
                    保存并生成
                  </Button>
                </div>
              ))}
          </footer>
        </DialogContent>
      </Dialog>
    </div>
  )
}
