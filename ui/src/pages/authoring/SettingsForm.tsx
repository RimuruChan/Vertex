import { useEffect, useState, type ReactNode } from 'react'
import type { DomainTreeEntry, DomainMaterialView } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Check, ChevronDown } from 'lucide-react'
import { Choice, Field, NumberField, TagsField } from './FormFields'
import { apiError } from '@/lib/format'

function Section({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <section className="grid gap-5 border-b border-border/70 pb-8 last:border-0 xl:grid-cols-[160px_minmax(0,1fr)]">
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        <p className="mt-2 text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <div className="min-w-0 max-w-3xl space-y-5">{children}</div>
    </section>
  )
}
export default function SettingsForm({
  problemId,
  revision,
  value,
  entries,
  disabled,
  onChange,
}: {
  problemId: string
  revision?: number
  value: Record<string, any>
  entries: DomainTreeEntry[]
  disabled: boolean
  onChange: (value: Record<string, any>) => void
}) {
  const api = useDomainAPI(),
    confirm = useConfirm()
  const [programs, setPrograms] = useState<DomainMaterialView[]>([]),
    [error, setError] = useState('')
  const programKey = entries
    .filter((e) => e.kind === 'program')
    .map((e) => e.id + e.blob.sha256)
    .join('|')
  useEffect(() => {
    let live = true
    void (async () => {
      const items: DomainMaterialView[] = []
      let after: string | undefined
      do {
        const page = await api.getApiAuthoringProblemsIdMaterials(problemId, {
          kind: 'program',
          limit: 100,
          after,
          revision,
        })
        items.push(...page.items)
        after = page.next
      } while (after && items.length < 1000)
      if (live) {
        setPrograms(items)
        setError('')
      }
    })().catch((e) => {
      if (live) setError(apiError(e, '程序列表加载失败'))
    })
    return () => {
      live = false
    }
  }, [api, problemId, revision, programKey])
  const change = (key: string, next: unknown) => onChange({ ...value, [key]: next })
  const roles = (role: string, current: string): [string, string][] => [
    ['', '暂不指定'],
    ...programs
      .filter((p) => p.program?.role === role || p.entry.id === current)
      .map(
        (p) =>
          [
            p.entry.id,
            `${p.program?.name ?? '未命名程序'}${p.program?.role !== role ? '（用途不匹配）' : ''}`,
          ] as [string, string],
      ),
    ...(current && !programs.some((p) => p.entry.id === current)
      ? [[current, '已引用的程序（尚未加载）'] as [string, string]]
      : []),
  ]
  const comparison = (next: Record<string, unknown>) =>
    change('comparison', { ...value.comparison, ...next })
  const field = (key: string, label: string, placeholder?: string) => (
    <Field label={label}>
      <Input
        maxLength={key === 'title' ? 200 : undefined}
        aria-invalid={key === 'title' && !String(value.title ?? '').trim()}
        aria-label={label}
        value={value[key] ?? ''}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => change(key, e.target.value)}
      />
      {key === 'title' && !String(value.title ?? '').trim() && (
        <p role="alert" className="text-xs text-destructive">
          请填写题目名称
        </p>
      )}
    </Field>
  )
  const rules = [
    ['tokens', '标准比较', '忽略多余空白，按内容判断答案'],
    ['exact', '严格比较', '包括空格和换行在内逐字节一致'],
    ['testlib', '自定义校验', '用于多解或特殊答案，选择校验器'],
  ] as const
  return (
    <div className="space-y-8">
      <Section title="题目信息" description="作者和参赛者看到的基本信息。">
        {field('title', '题目名称', '给题目起一个清晰的名字')}
        <div className="grid gap-4 sm:grid-cols-2">
          <Choice
            label="难度"
            disabled={disabled}
            value={String(value.difficulty)}
            onChange={(v) => change('difficulty', Number(v))}
            options={Array.from({ length: 10 }, (_, i) => [
              String(i + 1),
              `${i + 1} · ${['入门', '基础', '简单', '简单', '进阶', '进阶', '困难', '困难', '挑战', '挑战'][i]}`,
            ])}
          />
          <Choice
            label="默认题面"
            disabled={disabled}
            value={value.statementLanguage}
            onChange={(v) => change('statementLanguage', v)}
            options={[
              ...new Set(
                entries.filter((e) => e.kind === 'statement').map((e) => e.attributes.language),
              ),
            ].map((lang) => [
              lang,
              ({ zh: '中文', en: 'English' } as Record<string, string>)[lang] ?? lang,
            ])}
          />
        </div>
      </Section>
      <Section title="运行限制" description="提交程序可以使用的时间与内存。">
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField
            label="时间限制"
            integer
            unit="ms"
            value={value.timeLimitMs}
            disabled={disabled}
            min={1}
            max={3600000}
            onChange={(n) => change('timeLimitMs', n)}
          />
          <NumberField
            label="内存限制"
            unit="MiB"
            value={value.memoryLimitKb / 1024}
            disabled={disabled}
            min={1 / 1024}
            max={1048576}
            onChange={(n) => change('memoryLimitKb', Math.round(n * 1024))}
          />
        </div>
        <details className="group text-sm">
          <summary className="flex cursor-pointer list-none items-center gap-2 text-muted-foreground">
            <ChevronDown className="size-4 transition-transform group-open:rotate-180" />
            语言倍率与精确限制
          </summary>
          <div className="mt-4">
            <Choice
              label="限制方式"
              value={value.resourceMode || 'language-scaled'}
              disabled={disabled}
              onChange={(v) => change('resourceMode', v)}
              options={[
                ['language-scaled', '使用站点语言倍率'],
                ['exact', '所有语言使用上述精确限制'],
              ]}
            />
          </div>
        </details>
      </Section>
      <Section title="答案判定" description="先选比较方式，需要时再配置特殊规则。">
        <div className="grid gap-2 sm:grid-cols-3">
          {rules.map(([kind, title, hint]) => {
            const selected =
              kind === 'testlib'
                ? ['testlib', 'kattis'].includes(value.comparison.kind)
                : value.comparison.kind === kind
            return (
              <button
                key={kind}
                type="button"
                disabled={disabled}
                aria-pressed={selected}
                onClick={() => comparison({ kind })}
                className={`relative rounded-xl border p-4 text-left transition-colors ${selected ? 'border-primary bg-primary/5 ring-1 ring-primary/15' : 'bg-card hover:border-primary/40'}`}
              >
                <span className="block pr-4 text-sm font-medium">{title}</span>
                {selected && <Check className="absolute right-3 top-4 size-4 text-primary" />}
                <span className="mt-2 block text-xs leading-5 text-muted-foreground">{hint}</span>
              </button>
            )
          })}
        </div>
        {['testlib', 'kattis'].includes(value.comparison.kind) && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Choice
              label="校验器协议"
              value={value.comparison.kind}
              onChange={(kind) => comparison({ kind })}
              disabled={disabled}
              options={[
                ['testlib', 'testlib'],
                ['kattis', 'Kattis'],
              ]}
            />
            <Choice
              label="输出校验器"
              value={value.outputValidator}
              disabled={disabled}
              onChange={(v) => change('outputValidator', v)}
              options={roles('output-validator', value.outputValidator)}
            />
          </div>
        )}
        {value.comparison.kind === 'tokens' && (
          <details className="group">
            <summary className="flex cursor-pointer list-none items-center gap-2 text-sm text-muted-foreground">
              <ChevronDown className="size-4 transition-transform group-open:rotate-180" />
              空白、大小写与浮点误差
            </summary>
            <div className="mt-4 space-y-4 rounded-xl bg-muted/30 p-4">
              {(
                [
                  ['caseSensitive', '区分大小写'],
                  ['spaceSensitive', '区分空白差异'],
                  ['floatingPoint', '允许浮点误差'],
                ] as const
              ).map(([key, label]) => (
                <label
                  key={key}
                  className="flex cursor-pointer items-center justify-between gap-4 text-sm"
                >
                  <span>{label}</span>
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked={!!value.comparison[key]}
                    disabled={disabled}
                    onChange={(e) => comparison({ [key]: e.target.checked })}
                  />
                </label>
              ))}
              {value.comparison.floatingPoint && (
                <div className="grid gap-4 sm:grid-cols-2">
                  {(['absoluteTolerance', 'relativeTolerance'] as const).map((key, i) => (
                    <Field key={key} label={i ? '相对误差' : '绝对误差'}>
                      <Input
                        aria-label={i ? '相对误差' : '绝对误差'}
                        type="number"
                        min={0}
                        step="any"
                        value={value.comparison[key]}
                        disabled={disabled}
                        onChange={(e) => comparison({ [key]: Number(e.target.value) })}
                      />
                    </Field>
                  ))}
                </div>
              )}
            </div>
          </details>
        )}
      </Section>
      <Section title="数据质量" description="用正确参考解和输入校验器检查测试数据。">
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Choice
          label="主参考解"
          value={value.mainSolution}
          disabled={disabled}
          onChange={(v) => change('mainSolution', v)}
          options={roles('solution', value.mainSolution)}
        />
        <Field label="输入校验器" hint="可选择多个，检查时会全部运行。">
          <div className="divide-y rounded-xl border bg-card">
            {programs
              .filter(
                (p) =>
                  p.program?.role === 'input-validator' ||
                  value.inputValidators.includes(p.entry.id),
              )
              .map((p) => (
                <label
                  key={p.entry.id}
                  className="flex cursor-pointer items-center gap-3 p-3 text-sm"
                >
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    disabled={disabled}
                    checked={value.inputValidators.includes(p.entry.id)}
                    onChange={(e) =>
                      change(
                        'inputValidators',
                        e.target.checked
                          ? [...value.inputValidators, p.entry.id]
                          : value.inputValidators.filter((id: string) => id !== p.entry.id),
                      )
                    }
                  />
                  {p.program?.name}
                  {p.program?.role !== 'input-validator' && (
                    <span className="ml-auto text-xs text-amber-600">用途不匹配，可取消选择</span>
                  )}
                </label>
              ))}
            {!programs.some((p) => p.program?.role === 'input-validator') && (
              <p className="p-4 text-xs leading-5 text-muted-foreground">
                暂无输入校验器。可在“程序”中添加，普通参考解不会出现在此列表。
              </p>
            )}
          </div>
        </Field>
      </Section>
      <Section title="来源与授权" description="便于整理题库和保留原作者信息。">
        <div className="grid gap-4 sm:grid-cols-2">
          {field('source', '来源', '比赛或作者')}
          {field('rightsOwner', '权利人')}
          {field('license', '许可证', '例如 CC BY-SA 4.0')}
          <TagsField
            value={value.tags}
            disabled={disabled}
            onChange={(next) => change('tags', next)}
          />
        </div>
      </Section>
      {!!value.requirements?.length && (
        <Section title="导入兼容项" description="这些项目处理后才能继续交付。">
          {value.requirements.map((item: any, index: number) => (
            <div
              key={index}
              className="flex items-start gap-4 rounded-xl border border-amber-500/25 bg-amber-500/5 p-4"
            >
              <div className="min-w-0 flex-1">
                <p className="text-sm">{item.message}</p>
                <p className="mt-1 break-all text-xs text-muted-foreground">{item.path}</p>
              </div>
              <Button
                size="sm"
                variant="outline"
                disabled={disabled}
                onClick={async () => {
                  if (
                    await confirm({
                      title: '确认已完成处理？',
                      description: item.message,
                      confirmLabel: '已处理',
                    })
                  )
                    change(
                      'requirements',
                      value.requirements.filter((_: unknown, i: number) => i !== index),
                    )
                }}
              >
                标记已处理
              </Button>
            </div>
          ))}
        </Section>
      )}
    </div>
  )
}
