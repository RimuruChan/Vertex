export function metadataError(text: string): string {
  try {
    const value = JSON.parse(text)
    if (!String(value.title ?? '').trim()) return '请填写题目名称'
    if (
      !Number.isInteger(value.timeLimitMs) ||
      value.timeLimitMs < 1 ||
      value.timeLimitMs > 3600000
    )
      return '时间限制应为 1–3600000 ms'
    if (
      !Number.isInteger(value.memoryLimitKb) ||
      value.memoryLimitKb < 1 ||
      value.memoryLimitKb > 1073741824
    )
      return '请填写有效的内存限制'
    if (!Number.isInteger(value.difficulty) || value.difficulty < 1 || value.difficulty > 10)
      return '请选择难度'
    return ''
  } catch {
    return '设置内容尚未准备好'
  }
}
