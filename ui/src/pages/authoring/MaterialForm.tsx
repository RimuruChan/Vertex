import { Choice, Field } from './FormFields'
import SettingsForm from './SettingsForm'
import type { DomainTreeEntry } from '@/generated/api/model'
import { Input, Textarea } from '@/components/ui/input'

import { Button } from '@/components/ui/button'
import { entryLabel, roleNames, validationModes } from '@/lib/authoring-materials'

export { Choice, Field } from './FormFields'
// The raw JSON is the saved document; forms expose its typed fields without
// dropping fields the author is not currently editing.
export default function MaterialForm({
  kind,
  text,
  onChange,
  entries,
  disabled,
  problemId,
  revision,
}: {
  problemId: string
  revision?: number
  kind: string
  text: string
  onChange: (text: string) => void
  entries: DomainTreeEntry[]
  disabled: boolean
}) {
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
    <div
      className={kind === 'metadata' ? 'space-y-8' : 'max-w-3xl space-y-6 [&_textarea]:resize-none'}
    >
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
        <SettingsForm
          problemId={problemId}
          revision={revision}
          value={value}
          entries={entries}
          disabled={disabled}
          onChange={(next) => onChange(JSON.stringify(next, null, 2))}
        />
      )}
      {kind === 'program' && (
        <>
          {input('name', '程序名称')}
          <details className="rounded-lg border px-4 py-3">
            <summary className="cursor-pointer text-sm text-muted-foreground">
              工作目录与文件布局
            </summary>
            <div className="mt-4">
              {input(
                'directory',
                '程序目录',
                false,
                '通常保持默认。多文件程序可指定共同目录，保留相对引用。',
              )}
            </div>
          </details>
          <div className="grid gap-4 sm:grid-cols-2">
            {select(
              'role',
              '程序用途',
              Object.entries(roleNames).filter(
                ([id]) =>
                  ['solution', 'generator', 'input-validator', 'output-validator'].includes(id) ||
                  id === value.role,
              ),
            )}
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
