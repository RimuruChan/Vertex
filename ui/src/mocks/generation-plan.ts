import type { DomainGeneratedCase, DomainGenerationPlan } from '@/generated/api/model'
import { mockContentHash } from './workbench-checks'

export function generationArguments(text: string): string[] {
  const args: string[] = []
  let word = '',
    quote = '',
    escaped = false,
    started = false
  const flush = () => {
    if (started) args.push(word)
    word = ''
    started = false
  }
  for (const char of text) {
    if (char === '\0') throw new Error('参数不能包含空字符')
    if (escaped) {
      word += char
      escaped = false
      started = true
      continue
    }
    if (char === '\\' && quote !== "'") {
      escaped = true
      started = true
      continue
    }
    if (quote) {
      if (char === quote) quote = ''
      else word += char
      continue
    }
    if (char === "'" || char === '"') {
      quote = char
      started = true
      continue
    }
    if (/\s/u.test(char)) {
      flush()
      continue
    }
    word += char
    started = true
  }
  if (quote || escaped) throw new Error('参数的引号或转义没有结束')
  flush()
  if (args.length > 64 || args.some((a) => new TextEncoder().encode(a).length > 1024))
    throw new Error('参数过多或过长')
  return args
}
// Mock expansion only. Production previews always come from the server.
export function expandGeneration(id: string, plan: DomainGenerationPlan): DomainGeneratedCase[] {
  const validID = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/
  if (
    !validID.test(id) ||
    plan.schemaVersion !== 1 ||
    !plan.name.trim() ||
    !validID.test(plan.generator) ||
    !validID.test(plan.solution) ||
    !plan.rules.length ||
    plan.rules.length > 50
  )
    throw new Error('生成方案无效')
  const seen = new Set<string>(),
    cases: DomainGeneratedCase[] = []
  for (const rule of plan.rules) {
    if (
      !validID.test(rule.id) ||
      seen.has(rule.id) ||
      !rule.name.trim() ||
      !Number.isSafeInteger(rule.count) ||
      rule.count < 1 ||
      rule.count > 500 ||
      !Number.isSafeInteger(rule.seedStart) ||
      rule.seedStart < 0 ||
      rule.seedStart > 9007199254740000 ||
      rule.parameters.length > 8192
    )
      throw new Error('生成规则无效')
    seen.add(rule.id)
    const args = generationArguments(rule.parameters)
    for (let i = 0; i < rule.count; i++) {
      if (cases.length >= 500) throw new Error('一个生成方案最多 500 个测试点')
      const seed = rule.seedStart + i
      cases.push({
        id: 'generated-' + mockContentHash(`${id}\0${rule.id}\0${i}`).slice(0, 32),
        ruleId: rule.id,
        name: `${rule.name} · ${i + 1}`,
        seed,
        position: cases.length + 1,
        arguments: args.map((a) =>
          a
            .split('{seed}')
            .join(String(seed))
            .split('{index}')
            .join(String(i + 1)),
        ),
      })
    }
  }
  return cases
}
