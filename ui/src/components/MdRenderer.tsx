import { useMemo } from 'react'
import MarkdownIt from 'markdown-it'
import katex from 'katex'
import DOMPurify from 'dompurify'
import 'katex/dist/katex.min.css'
import { cn } from '@/lib/utils'

// 自定义 dollarmath 插件:把 $...$ 与 $$...$$ 渲染为 KaTeX。
// 与 markdown-it-dollarmath 等价,避免其老版本 peer 依赖。
function dollarmathPlugin(md: MarkdownIt) {
  // 行内 $...$
  const inlineRule = (state: any) => {
    const src = state.src
    const start = state.pos
    if (src[start] !== '$') return false
    // 跳过 $$(块级处理)
    if (src[start + 1] === '$') return false
    let end = start + 1
    while (end < src.length) {
      if (src[end] === '$') break
      end++
    }
    if (end >= src.length) return false
    const content = src.slice(start + 1, end)
    if (!content.trim()) return false
    try {
      const html = katex.renderToString(content, {
        throwOnError: false,
        displayMode: false,
      })
      const token = state.push('html_inline', '', 0)
      token.content = html
      state.pos = end + 1
      return true
    } catch {
      return false
    }
  }
  md.inline.ruler.before('escape', 'dollarmath', inlineRule)

  // 块级 $$...$$
  const blockRule = (state: any, startLine: number) => {
    const pos = state.bMarks[startLine] + state.tShift[startLine]
    const line = state.src.slice(pos, state.eMarks[startLine]).trim()
    if (!line.startsWith('$$')) return false

    const collected: string[] = []
    let lineNo = startLine
    let found = false
    const firstContent = line.slice(2).trim()
    if (firstContent.endsWith('$$') && firstContent.length > 2) {
      collected.push(firstContent.slice(0, -2))
      found = true
    } else if (firstContent.length > 0) {
      collected.push(firstContent)
    }
    if (!found) {
      for (lineNo = startLine + 1; lineNo < state.lineMax; lineNo++) {
        const nextPos = state.bMarks[lineNo] + state.tShift[lineNo]
        const nextLine = state.src.slice(nextPos, state.eMarks[lineNo]).trim()
        if (nextLine.endsWith('$$')) {
          collected.push(nextLine.slice(0, -2))
          found = true
          break
        }
        collected.push(nextLine)
      }
    }
    if (!found) return false

    const content = collected.join('\n')
    try {
      const html = katex.renderToString(content, {
        throwOnError: false,
        displayMode: true,
      })
      const token = state.push('html_block', '', 0)
      token.content = html
      state.line = lineNo + 1
      return true
    } catch {
      return false
    }
  }
  md.block.ruler.before('paragraph', 'dollarmath_block', blockRule, {
    alt: ['paragraph', 'reference'],
  })
}

const md = new MarkdownIt({
  html: false, // 原始 HTML 禁止,防 XSS
  linkify: true,
  breaks: true,
})

md.use(dollarmathPlugin)

// 渲染 markdown → 安全 HTML(DOMPurify 二次消毒)
function markdownURLs(src: string, kind: 'image' | 'link_open', attribute: string): string[] {
  const found = new Set<string>()
  const walk = (tokens: ReturnType<typeof md.parse>) => {
    for (const token of tokens) {
      if (token.type === kind) {
        const source = token.attrGet(attribute)
        if (source) found.add(source)
      }
      if (token.children) walk(token.children)
    }
  }
  walk(md.parse(src || '', {}))
  return [...found]
}
export const markdownImages = (source: string) => markdownURLs(source, 'image', 'src')
export const markdownLinks = (source: string) => markdownURLs(source, 'link_open', 'href')
type MediaURLs = { images: Record<string, string>; links: Record<string, string> }
function renderMd(src: string, media?: MediaURLs): string {
  if (!media) return DOMPurify.sanitize(md.render(src || ''))
  // Sanitize authored markup first. Only authenticated object URLs supplied by
  // the publication component are installed after sanitization.
  const fragment = DOMPurify.sanitize(md.render(src || ''), { RETURN_DOM_FRAGMENT: true })
  for (const node of fragment.querySelectorAll('img[src]')) {
    const value = media.images[node.getAttribute('src') ?? '']
    if (value !== undefined) {
      if (value) node.setAttribute('src', value)
      else node.removeAttribute('src')
    }
  }
  for (const node of fragment.querySelectorAll('a[href]')) {
    const value = media.links[node.getAttribute('href') ?? '']
    if (value !== undefined) node.setAttribute('href', value)
  }
  const holder = document.createElement('div')
  holder.append(fragment)
  return holder.innerHTML
}

// MdRenderer:题面/题解/评论的 Markdown 渲染组件。
// 预计算 HTML 避免每帧重渲染;dangerouslySetInnerHTML 的输入已被消毒。
// 具体排版样式见 index.css 的 .markdown-body。
export default function MdRenderer({
  content,
  className,
  media,
}: {
  content: string
  className?: string
  media?: MediaURLs
}) {
  const html = useMemo(() => renderMd(content, media), [content, media])
  return (
    <div className={cn('markdown-body', className)} dangerouslySetInnerHTML={{ __html: html }} />
  )
}
