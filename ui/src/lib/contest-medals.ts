export type MedalConfig = {
  mode: 'none' | 'count' | 'percentage'
  gold: number
  silver: number
  bronze: number
}
export type Medal = 'gold' | 'silver' | 'bronze'
type MedalRow = { rank: number; solved: number; medal?: Medal }

export function defaultMedals(format: string): MedalConfig {
  return format === 'icpc'
    ? { mode: 'percentage', gold: 10, silver: 20, bronze: 30 }
    : { mode: 'none', gold: 0, silver: 0, bronze: 0 }
}

export function medalSettings(value: unknown): MedalConfig {
  if (value === undefined || value === null) return { mode: 'none', gold: 0, silver: 0, bronze: 0 }
  if (typeof value !== 'object' || Array.isArray(value)) throw new Error('奖牌设置无效')
  const data = value as Partial<MedalConfig>
  const result = {
    mode: data.mode ?? 'none',
    gold: data.gold ?? 0,
    silver: data.silver ?? 0,
    bronze: data.bronze ?? 0,
  }
  if (!['none', 'count', 'percentage'].includes(result.mode))
    throw new Error('请选择有效的奖牌分配方式')
  if (
    [result.gold, result.silver, result.bronze].some(
      (n) => !Number.isSafeInteger(n) || n < 0 || n > 100000,
    )
  )
    throw new Error('奖牌名额须为 0–100000 的整数')
  if (result.mode === 'percentage' && result.gold + result.silver + result.bronze > 100)
    throw new Error('金银铜比例合计不能超过 100%')
  return result
}

export function assignMedals(rows: MedalRow[], config: MedalConfig) {
  for (const row of rows) delete row.medal
  if (config.mode === 'none') return undefined
  const eligible = rows.filter((row) => row.solved > 0).length
  const quotas = [config.gold, config.silver, config.bronze].map((value) =>
    config.mode === 'percentage' ? Math.floor((eligible * value) / 100) : value,
  )
  const summary = { mode: config.mode, eligible, gold: 0, silver: 0, bronze: 0 }
  let position = 0,
    groupPosition = 0,
    priorRank = -1
  for (const row of rows) {
    if (row.solved <= 0) continue
    position++
    if (row.rank !== priorRank) {
      groupPosition = position
      priorRank = row.rank
    }
    if (groupPosition <= quotas[0]) row.medal = 'gold'
    else if (groupPosition <= quotas[0] + quotas[1]) row.medal = 'silver'
    else if (groupPosition <= quotas[0] + quotas[1] + quotas[2]) row.medal = 'bronze'
    if (row.medal) summary[row.medal]++
  }
  return summary
}
