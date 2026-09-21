export function statementSections(content: string): Record<string, string> {
  const result: Record<string, string> = Object.create(null)
  let key = '题目描述',
    fence = ''
  for (const line of content.replace(/\r\n/g, '\n').split('\n')) {
    const trimmed = line.trim()
    if (/^(```|~~~)/.test(trimmed)) {
      if (!fence) fence = trimmed.slice(0, 3)
      else if (trimmed.startsWith(fence)) fence = ''
    }
    const heading = !fence && trimmed.match(/^(#+)\s+(.+)$/)
    if (heading) {
      if (heading[1] === '#') {
        result['题目标题'] = heading[2].trim()
        key = '题目描述'
        continue
      }
      key = heading[2].trim()
      result[key] ??= ''
      continue
    }
    result[key] = (result[key] ?? '') + line + '\n'
  }
  return Object.fromEntries(Object.entries(result).map(([key, value]) => [key, value.trim()]))
}
