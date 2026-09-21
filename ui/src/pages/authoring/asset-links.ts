import type { DomainTreeEntry } from '@/generated/api/model'
export function attachmentMarkdown(statement: DomainTreeEntry, file: DomainTreeEntry) {
  const from = statement.path.split('/').slice(0, -1),
    to = file.path.split('/')
  while (from.length && to.length && from[0] === to[0]) {
    from.shift()
    to.shift()
  }
  const parts = [...from.map(() => '..'), ...to]
  if (statement.attributes.format === 'tex') {
    const relative = parts.join('/')
    if (!/\.(png|jpe?g|pdf)$/i.test(file.path) || /[{}%#^\\\r\n]/.test(relative))
      throw new Error('此文件不适合直接插入 TeX，请使用文件名不含特殊符号的 PNG、JPEG 或 PDF。')
    return `\\includegraphics[width=.8\\linewidth]{\\detokenize{${relative}}}`
  }
  const relative = parts
    .map((part) =>
      encodeURIComponent(part).replace(/[()']/g, (c) => '%' + c.charCodeAt(0).toString(16)),
    )
    .join('/')
  const label = (file.attributes.label || file.path.split('/').pop()!).replace(/[\\[\]]/g, '\\$&')
  return `${/\.(png|jpe?g|webp|gif)$/i.test(file.path) ? '!' : ''}[${label}](${relative})`
}
