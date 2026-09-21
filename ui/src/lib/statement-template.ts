export function standardStatement(title: string, language = 'zh') {
  const name = title.replace(/[\r\n]/g, ' ')
  const headings = language.startsWith('zh')
    ? ['题目描述', '输入格式', '输出格式', '样例说明', '数据范围与提示']
    : ['Description', 'Input', 'Output', 'Explanation', 'Constraints']
  return `# ${name}\n\n## ${headings[0]}\n\n\n## ${headings[1]}\n\n\n## ${headings[2]}\n\n\n{{remainingsamples}}\n\n## ${headings[3]}\n\n\n## ${headings[4]}\n\n`
}
