// Reproduce placement without executing code. Inserted testcase bytes are never
// rescanned for markup commands, and fences/inline code remain literal.
export function placeMockSamples(content: string, samples: string[]) {
  let used = 0,
    fence = '',
    inline = '',
    output = ''
  for (const line of content.split('\n')) {
    const marker = line.match(/^ {0,3}(`{3,}|~{3,})/)
    if (marker) {
      if (!fence) fence = marker[1]
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = ''
      output += line + '\n'
      continue
    }
    if (fence || /^( {4}|\t)/.test(line)) {
      output += line + '\n'
      continue
    }
    let last = 0
    for (const match of line.matchAll(/`+|\{\{(?:nextsample|remainingsamples)\}\}/g)) {
      const index = match.index!,
        token = match[0]
      let slashes = 0
      for (let i = index - 1; i >= 0 && line[i] === '\\'; i--) slashes++
      if (slashes % 2) continue
      if (token[0] === '`') {
        if (!inline) inline = token
        else if (inline === token) inline = ''
        continue
      }
      if (inline) continue
      output += line.slice(last, index) + '\n\n'
      if (token === '{{nextsample}}') {
        if (used >= samples.length) throw new Error('题面中的样例位置超过可用样例数量')
        output += samples[used++]
      } else {
        output += samples.slice(used).join('\n\n')
        used = samples.length
      }
      output += '\n\n'
      last = index + token.length
    }
    output += line.slice(last) + '\n'
  }
  if (used < samples.length) output += '\n' + samples.slice(used).join('\n\n')
  return output
}
export function sampleMarkdown(index: number, input: string, answer: string, language = 'zh') {
  const fence = (text: string) => {
    const mark = '`'.repeat(Math.max(3, ...(text.match(/`+/g) ?? []).map((s) => s.length + 1)))
    return `${mark}\n${text.trimEnd()}\n${mark}`
  }
  const zh = language.startsWith('zh')
  return `## ${zh ? '样例' : 'Example'} ${index}\n\n**${zh ? '输入' : 'Input'}**\n\n${fence(input)}\n\n**${zh ? '输出' : 'Output'}**\n\n${fence(answer)}`
}
