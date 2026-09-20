import type { ReactNode } from 'react'
import type { DomainTreeEntry } from '@/generated/api/model'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { entryLabel, roleNames, validationModes } from '@/lib/authoring-materials'

export function Choice({
  label,
  value,
  onChange,
  options,
  disabled = false,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: [string, string][]
  disabled?: boolean
}) {
  return (
    <Field label={label}>
      <Select
        value={value || '__none'}
        onValueChange={(value) => onChange(value === '__none' ? '' : value)}
        disabled={disabled}
      >
        <SelectTrigger aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(([id, name]) => (
            <SelectItem key={id || '__none'} value={id || '__none'}>
              {name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}
export function Field({
  label,
  children,
  hint,
}: {
  label: string
  children: ReactNode
  hint?: string
}) {
  return (
    <div className="min-w-0 space-y-1.5">
      <Label>{label}</Label>
      {children}
      {hint && <p className="text-xs leading-5 text-muted-foreground">{hint}</p>}
    </div>
  )
}

// The raw JSON is the saved document; forms expose its typed fields without
// dropping fields the author is not currently editing.
export default function MaterialForm({
  kind,
  text,
  onChange,
  entries,
  disabled,
}: {
  kind: string
  text: string
  onChange: (text: string) => void
  entries: DomainTreeEntry[]
  disabled: boolean
}) {
  const confirm = useConfirm()
  let value: Record<string, any>
  try {
    value = JSON.parse(text)
    if (!value || typeof value !== 'object' || Array.isArray(value) || value.schemaVersion !== 1)
      throw new Error('invalid document')
    const arrays =
      kind === 'metadata'
        ? ['tags', 'inputValidators', 'testOrder']
        : kind === 'program'
          ? ['files', 'arguments', 'expectedVerdicts']
          : kind === 'group'
            ? ['prerequisites']
            : []
    if (arrays.some((key) => !Array.isArray(value[key]))) throw new Error('missing fields')
    if (
      (kind === 'metadata' && !value.comparison) ||
      (kind === 'test' && (!value.input || !Array.isArray(value.input.arguments) || !value.answer))
    )
      throw new Error('missing fields')
  } catch {
    return (
      <div className="space-y-3">
        <p role="alert" className="text-sm text-destructive">
          材料暂时无法显示为表单，可修复原始文档或从历史恢复。
        </p>
        <Textarea
          aria-label="原始材料文档"
          className="min-h-80 font-mono"
          value={text}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
        />
      </div>
    )
  }
  const change = (key: string, next: unknown) => {
    const updated = { ...value, [key]: next }
    if (kind === 'validation' && key === 'mode' && next === 'invalid_input') {
      updated.answer = ''
      updated.output = ''
    }
    if (kind === 'program' && key === 'role') {
      updated.expectedVerdicts = next === 'solution' ? ['Accepted'] : []
      updated.protocol = next === 'solution' || next === 'generator' ? 'stdio' : 'testlib'
    }
    onChange(JSON.stringify(updated, null, 2))
  }
  const input = (key: string, label: string, numeric = false, hint?: string) => (
    <Field label={label} hint={hint}>
      <Input
        aria-label={label}
        disabled={disabled}
        type={numeric ? 'number' : 'text'}
        value={value[key] ?? ''}
        onChange={(event) => change(key, numeric ? Number(event.target.value) : event.target.value)}
      />
    </Field>
  )
  const select = (key: string, label: string, options: [string, string][]) => (
    <Choice
      label={label}
      value={value[key] || ''}
      onChange={(next) => change(key, next)}
      options={options}
      disabled={disabled}
    />
  )
  const refs = (kind: string): [string, string][] => [
    ['', '未指定'],
    ...entries
      .filter((entry) => entry.kind === kind)
      .map(
        (entry) =>
          [
            entry.id,
            ['source', 'input', 'answer'].includes(kind) ? entry.path : entryLabel(entry),
          ] as [string, string],
      ),
  ]
  const toggle = (key: string, label: string) => (
    <Button
      variant={value[key] ? 'secondary' : 'outline'}
      aria-pressed={Boolean(value[key])}
      disabled={disabled}
      onClick={() => change(key, !value[key])}
    >
      {label}
    </Button>
  )
  const nested = (key: string, field: string, next: unknown) =>
    change(key, { ...value[key], [field]: next })
  return (
    <div className="max-w-3xl space-y-6">
      {kind === 'validation' && (
        <div className="space-y-5">
          <p className="text-sm leading-6 text-muted-foreground">
            用已知的正确和错误数据检验校验器本身。这些文件只用于出题检查，不进入选手评测或公开下载。
          </p>
          <div className="grid gap-5 sm:grid-cols-2">
            {input('name', '自测名称')}
            {select('mode', '预期行为', validationModes)}
            {select('input', '输入文件', refs('input'))}
            {value.mode !== 'invalid_input' && select('answer', '标准答案', refs('answer'))}
            {value.mode !== 'invalid_input' && select('output', '待验证输出', refs('answer'))}
          </div>
          <Field
            label="自测说明"
            hint={
              value.mode === 'invalid_input'
                ? '至少一个输入校验器应正常拒绝；崩溃或超限不算通过。'
                : '输入必须先通过输入校验，再验证候选输出是否符合预期。'
            }
          >
            <Textarea
              aria-label="自测说明"
              disabled={disabled}
              value={value.description ?? ''}
              onChange={(event) => change('description', event.target.value)}
              placeholder="这组数据要验证哪种边界情况？"
            />
          </Field>
        </div>
      )}
      {kind === 'metadata' && (
        <>
          {Array.isArray(value.requirements) && value.requirements.length > 0 && (
            <section
              className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-4 space-y-3"
              aria-label="待处理的兼容要求"
            >
              <div>
                <h3 className="font-medium text-sm">导入材料需要确认</h3>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  以下配置已保留原文件，但还未转换为可执行规则。处理前会阻止检查或发布。
                </p>
              </div>
              {value.requirements.map(
                (
                  item: { code: string; path?: string; message: string; stage: string },
                  index: number,
                ) => (
                  <div
                    key={`${item.code}:${index}`}
                    className="flex flex-wrap items-start justify-between gap-2 border-t border-border/60 pt-3"
                  >
                    <div className="min-w-0 flex-1 text-sm">
                      <p>{item.message}</p>
                      <p className="mt-1 break-all text-xs text-muted-foreground">
                        {item.path} · {item.stage === 'build' ? '检查前处理' : '发布前处理'}
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={disabled}
                      onClick={async () => {
                        if (
                          await confirm({
                            title: '确认已处理这项配置？',
                            description: `${item.message}。请先完成相应材料的修改；确认后这项阻断会随下一次保存移除。`,
                            confirmLabel: '已处理，移除阻断',
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
                ),
              )}
            </section>
          )}
          {input('title', '题目名称')}
          {input('difficulty', '难度（1–10）', true)}
          <div className="grid gap-4 sm:grid-cols-2">
            {input('timeLimitMs', '时间限制（ms）', true)}
            {input('memoryLimitKb', '内存限制（KiB）', true)}
            {select('resourceMode', '资源限制方式', [
              ['language-scaled', '按站点语言倍率'],
              ['exact', '使用精确限制（标准题包）'],
            ])}
            {select(
              'statementLanguage',
              '默认题面语言',
              [
                ...new Set(
                  entries
                    .filter((e) => e.kind === 'statement')
                    .map((e) => e.attributes.language || 'zh'),
                ),
              ].map((language) => [language, language]),
            )}
            {select('judgeType', '题目类型', [
              ['normal', '传统题'],
              ['interactive', '交互题'],
            ])}
          </div>
          <div className="space-y-4 border-t pt-5">
            <h3 className="text-sm font-medium">判定与参考程序</h3>
            <Choice
              label="输出比较方式"
              value={value.comparison.kind}
              disabled={disabled}
              onChange={(kind) =>
                change('comparison', {
                  ...value.comparison,
                  kind,
                  floatingPoint: false,
                  absoluteTolerance: 0,
                  relativeTolerance: 0,
                })
              }
              options={[
                ['tokens', '按单词比较'],
                ['exact', '精确比较'],
                ['testlib', 'testlib checker'],
                ['kattis', 'Kattis output validator'],
              ]}
            />
            {value.comparison.kind === 'tokens' && (
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2 sm:col-span-2">
                  <Button
                    variant={value.comparison.floatingPoint ? 'secondary' : 'outline'}
                    aria-pressed={Boolean(value.comparison.floatingPoint)}
                    disabled={disabled}
                    onClick={() =>
                      change('comparison', {
                        ...value.comparison,
                        floatingPoint: !value.comparison.floatingPoint,
                        absoluteTolerance: 0,
                        relativeTolerance: 0,
                      })
                    }
                  >
                    浮点数比较
                  </Button>
                  <p className="text-xs text-muted-foreground">
                    启用后按数值比较；误差为 0 时仍允许 1 和 1.0 等不同写法。
                  </p>
                </div>
                <Field label="绝对误差">
                  <Input
                    aria-label="绝对误差"
                    disabled={disabled || !value.comparison.floatingPoint}
                    type="number"
                    min="0"
                    step="any"
                    value={value.comparison.absoluteTolerance}
                    onChange={(e) =>
                      nested('comparison', 'absoluteTolerance', Number(e.target.value))
                    }
                  />
                </Field>
                <Field label="相对误差">
                  <Input
                    aria-label="相对误差"
                    disabled={disabled || !value.comparison.floatingPoint}
                    type="number"
                    min="0"
                    step="any"
                    value={value.comparison.relativeTolerance}
                    onChange={(e) =>
                      nested('comparison', 'relativeTolerance', Number(e.target.value))
                    }
                  />
                </Field>
                <Button
                  disabled={disabled}
                  aria-pressed={value.comparison.caseSensitive}
                  variant={value.comparison.caseSensitive ? 'secondary' : 'outline'}
                  onClick={() =>
                    nested('comparison', 'caseSensitive', !value.comparison.caseSensitive)
                  }
                >
                  区分大小写
                </Button>
                <Button
                  disabled={disabled}
                  aria-pressed={value.comparison.spaceSensitive}
                  variant={value.comparison.spaceSensitive ? 'secondary' : 'outline'}
                  onClick={() =>
                    nested('comparison', 'spaceSensitive', !value.comparison.spaceSensitive)
                  }
                >
                  区分空白差异
                </Button>
              </div>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              {select('mainSolution', '主参考解', refs('program'))}
              {select('outputValidator', '输出校验器', refs('program'))}
            </div>
            <Field label="输入校验器">
              <div className="flex flex-wrap gap-2">
                {entries
                  .filter((e) => e.kind === 'program')
                  .map((entry) => (
                    <Button
                      key={entry.id}
                      disabled={disabled}
                      aria-pressed={value.inputValidators.includes(entry.id)}
                      variant={value.inputValidators.includes(entry.id) ? 'secondary' : 'outline'}
                      onClick={() =>
                        change(
                          'inputValidators',
                          value.inputValidators.includes(entry.id)
                            ? value.inputValidators.filter((id: string) => id !== entry.id)
                            : [...value.inputValidators, entry.id],
                        )
                      }
                    >
                      {entryLabel(entry)}
                    </Button>
                  ))}
                {!entries.some((e) => e.kind === 'program') && (
                  <p className="text-sm text-muted-foreground">在程序页添加校验器后选择。</p>
                )}
              </div>
            </Field>
          </div>
          <div className="grid gap-4 border-t pt-5 sm:grid-cols-2">
            {input('source', '来源')}
            {input('license', '许可证')}
            {input('rightsOwner', '权利人')}
            <Field label="标签">
              <Input
                aria-label="标签"
                disabled={disabled}
                value={value.tags.join(', ')}
                onChange={(e) =>
                  change(
                    'tags',
                    e.target.value
                      .split(',')
                      .map((s) => s.trim())
                      .filter(Boolean),
                  )
                }
              />
            </Field>
          </div>
        </>
      )}
      {kind === 'program' && (
        <>
          {input('name', '程序名称')}
          {input(
            'directory',
            '程序目录',
            false,
            '留空从题目根目录运行；程序中的相对路径以此目录为基准。',
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            {select('role', '程序用途', Object.entries(roleNames))}
            {select('language', '语言', [
              ['cpp', 'C++'],
              ['c', 'C'],
              ['python', 'Python'],
              ['java', 'Java'],
              ['rust', 'Rust'],
            ])}
            {select('protocol', '运行协议', [
              ['stdio', '标准输入输出'],
              ['testlib', 'testlib'],
              ['kattis', 'Kattis'],
            ])}
            {select(
              'entryPoint',
              '入口文件',
              refs('source').filter(([id]) => !id || value.files.includes(id)),
            )}
          </div>
          <Field label="程序文件" hint="可选择多个源文件组成一个程序，文件改名不会破坏引用。">
            <div className="flex flex-wrap gap-2">
              {entries
                .filter((e) => e.kind === 'source')
                .map((entry) => (
                  <Button
                    key={entry.id}
                    disabled={disabled}
                    aria-pressed={value.files.includes(entry.id)}
                    variant={value.files.includes(entry.id) ? 'secondary' : 'outline'}
                    onClick={() => {
                      const files = value.files.includes(entry.id)
                        ? value.files.filter((id: string) => id !== entry.id)
                        : [...value.files, entry.id]
                      onChange(
                        JSON.stringify(
                          {
                            ...value,
                            files,
                            entryPoint: files.includes(value.entryPoint)
                              ? value.entryPoint
                              : files[0] || '',
                          },
                          null,
                          2,
                        ),
                      )
                    }}
                  >
                    {entry.path}
                  </Button>
                ))}
              {!entries.some((e) => e.kind === 'source') && (
                <p className="text-sm text-muted-foreground">先添加源代码文件。</p>
              )}
            </div>
          </Field>
          <Field label="运行参数" hint="每行一个参数，直接传入程序，不通过 shell。">
            <Textarea
              aria-label="运行参数"
              disabled={disabled}
              value={value.arguments.join('\n')}
              onChange={(e) =>
                change('arguments', e.target.value ? e.target.value.split('\n') : [])
              }
            />
          </Field>
          {value.role === 'solution' && (
            <Field label="预期结果">
              <div className="flex flex-wrap gap-2">
                {[
                  ['Accepted', '通过'],
                  ['Wrong Answer', '答案错误'],
                  ['Time Limit Exceeded', '超时'],
                  ['Runtime Error', '运行错误'],
                  ['Any Rejection', '任意不通过'],
                ].map(([id, name]) => (
                  <Button
                    key={id}
                    disabled={disabled}
                    aria-pressed={value.expectedVerdicts.includes(id)}
                    variant={value.expectedVerdicts.includes(id) ? 'secondary' : 'outline'}
                    onClick={() =>
                      change(
                        'expectedVerdicts',
                        value.expectedVerdicts.includes(id)
                          ? value.expectedVerdicts.filter((v: string) => v !== id)
                          : [...value.expectedVerdicts, id],
                      )
                    }
                  >
                    {name}
                  </Button>
                ))}
              </div>
            </Field>
          )}
        </>
      )}
      {kind === 'test' && (
        <>
          <div className="grid gap-4 sm:grid-cols-2">
            {input('name', '测试点名称')}
            {select('group', '所属分组', refs('group'))}
          </div>
          <div className="flex flex-wrap gap-2">
            {toggle('isSample', '作为样例')}
            {toggle('isPretest', '预测试')}
          </div>
          <div className="grid gap-4 border-t pt-5 sm:grid-cols-2">
            <Choice
              label="输入来源"
              value={value.input.kind}
              onChange={(kind) =>
                change('input', { kind, entry: '', generator: '', arguments: [] })
              }
              options={[
                ['file', '已有输入文件'],
                ['generator', '生成器'],
              ]}
              disabled={disabled}
            />
            <Choice
              label={value.input.kind === 'file' ? '输入文件' : '生成器'}
              value={value.input.kind === 'file' ? value.input.entry : value.input.generator}
              onChange={(id) =>
                nested('input', value.input.kind === 'file' ? 'entry' : 'generator', id)
              }
              options={refs(value.input.kind === 'file' ? 'input' : 'program')}
              disabled={disabled}
            />
            <Choice
              label="答案来源"
              value={value.answer.kind}
              onChange={(kind) => change('answer', { kind, entry: '', solution: '' })}
              options={[
                ['file', '保留已有答案'],
                ['solution', '参考解生成'],
              ]}
              disabled={disabled}
            />
            <Choice
              label={value.answer.kind === 'file' ? '答案文件' : '答案生成程序'}
              value={value.answer.kind === 'file' ? value.answer.entry : value.answer.solution}
              onChange={(id) =>
                nested('answer', value.answer.kind === 'file' ? 'entry' : 'solution', id)
              }
              options={refs(value.answer.kind === 'file' ? 'answer' : 'program')}
              disabled={disabled}
            />
          </div>
          {value.input.kind === 'generator' && (
            <Field label="生成参数" hint="每行一个参数，种子也应明确写入参数。">
              <Textarea
                aria-label="生成参数"
                disabled={disabled}
                value={value.input.arguments.join('\n')}
                onChange={(e) =>
                  nested('input', 'arguments', e.target.value ? e.target.value.split('\n') : [])
                }
              />
            </Field>
          )}
          <div className="grid gap-4 sm:grid-cols-3">
            {input('points', '分值', true)}
            {input('timeLimitMs', '独立时限（ms）', true, '0 表示使用题目限制')}
            {input('memoryLimitKb', '独立内存（KiB）', true, '0 表示使用题目限制')}
          </div>
          <Field label="测试点说明">
            <Textarea
              aria-label="测试点说明"
              disabled={disabled}
              value={value.description}
              onChange={(e) => change('description', e.target.value)}
            />
          </Field>
        </>
      )}
      {kind === 'group' && (
        <>
          {input('name', '分组名称')}
          <div className="grid gap-4 sm:grid-cols-2">
            {select('aggregation', '组内计分方式', [
              ['pass-fail', '全部通过'],
              ['sum', '分值求和'],
              ['min', '最低分'],
            ])}
            {input('maxScore', '组最高分', true)}
          </div>
          <Field label="依赖的测试组">
            <div className="flex flex-wrap gap-2">
              {entries
                .filter((e) => e.kind === 'group')
                .map((entry) => (
                  <Button
                    key={entry.id}
                    disabled={disabled}
                    aria-pressed={value.prerequisites.includes(entry.id)}
                    variant={value.prerequisites.includes(entry.id) ? 'secondary' : 'outline'}
                    onClick={() =>
                      change(
                        'prerequisites',
                        value.prerequisites.includes(entry.id)
                          ? value.prerequisites.filter((id: string) => id !== entry.id)
                          : [...value.prerequisites, entry.id],
                      )
                    }
                  >
                    {entryLabel(entry)}
                  </Button>
                ))}
            </div>
          </Field>
          <Field label="分组说明">
            <Textarea
              aria-label="分组说明"
              disabled={disabled}
              value={value.description}
              onChange={(e) => change('description', e.target.value)}
            />
          </Field>
        </>
      )}
    </div>
  )
}
