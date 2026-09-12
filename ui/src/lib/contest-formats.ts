export const contestFormatOptions = [
  ['icpc', 'ICPC · 通过题数与罚时'],
  ['oi', 'OI · 最后提交，赛后公布'],
  ['ioi', 'IOI · 每题最高分'],
  ['leduo', '乐多 · 提交次数折扣'],
  ['cf', 'CF 积分 · 时间与错误扣分'],
] as const

export function isScoreContest(format: string | undefined) {
  return ['oi', 'ioi', 'leduo', 'cf'].includes(format ?? '')
}

export function contestFormatName(format: string) {
  return format === 'leduo' ? '乐多' : format === 'cf' ? 'CF 积分' : format.toUpperCase()
}

export const contestFormatDescription: Record<string, string> = {
  icpc: '通过题数优先，同题数比较罚时；默认仅公布最终判定，不展示逐测试点详情。',
  oi: '每题以最后一次有效提交计分。比赛中不公布正式结果或公开榜单，赛后统一开放；封榜计划在 OI 中不生效。',
  ioi: '每题取历次提交最高分，支持部分分；预设完整测试点反馈。官方 IOI 的子任务得分与每组首错摘要尚不在此模式内。',
  leduo:
    '每次有效提交按 0.95^(次数−1) 折算，最低为 70%，取折算后最高分。编译错误计入次数；预设完整反馈，可单独调整。',
  cf: '通过题目得分，时间衰减、错误提交扣 50 分，最低保留 30%。默认仅反馈首个失败点编号和类型。直接完整评测，不含 Hack 或预测试；编译错误不扣分。',
}
