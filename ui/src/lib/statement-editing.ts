import type { EditorView } from '@codemirror/view'
import { EditorSelection } from '@codemirror/state'

export type StatementFormat =
  'bold' | 'italic' | 'heading' | 'list' | 'quote' | 'code' | 'formula' | 'ordered' | 'strike'

/** Source edits preserve everything outside the selection, including imported markup. */
export function formatStatement(view: EditorView, format: StatementFormat, tex = false, level = 2) {
  const { from, to } = view.state.selection.main
  if (
    !tex &&
    (format === 'heading' || format === 'list' || format === 'quote' || format === 'ordered')
  ) {
    const first = view.state.doc.lineAt(from)
    const last = view.state.doc.lineAt(
      to > from && view.state.doc.sliceString(to - 1, to) === '\n' ? to - 1 : to,
    )
    const lines = view.state.doc.sliceString(first.from, last.to).split('\n')
    const marker = {
      heading: '#'.repeat(Math.max(1, Math.min(6, level))) + ' ',
      list: '- ',
      quote: '> ',
      ordered: '1. ',
    }[format]!
    const remove = lines.every((line) =>
      format === 'ordered' ? /^\d+\.\s/.test(line) : line.startsWith(marker),
    )
    const insert = lines
      .map((line, index) =>
        remove
          ? format === 'ordered'
            ? line.replace(/^\d+\.\s+/, '')
            : line.slice(marker.length)
          : (format === 'ordered' ? `${index + 1}. ` : marker) +
            (format === 'heading' ? line.replace(/^#{1,6}\s+/, '') : line),
      )
      .join('\n')
    view.dispatch({
      changes: { from: first.from, to: last.to, insert },
      selection: EditorSelection.range(first.from, first.from + insert.length),
      userEvent: 'input.format',
    })
  } else {
    const pairs: Record<StatementFormat, [string, string, string]> = tex
      ? {
          strike: ['\\sout{', '}', '文字'],
          ordered: ['\\begin{enumerate}\n\\item ', '\n\\end{enumerate}', '列表项'],
          bold: ['\\textbf{', '}', '文字'],
          italic: ['\\textit{', '}', '文字'],
          heading: [
            level <= 1 ? '\\section{' : level === 2 ? '\\subsection{' : '\\subsubsection{',
            '}',
            '标题',
          ],
          list: ['\\begin{itemize}\n\\item ', '\n\\end{itemize}', '列表项'],
          quote: ['\\begin{quote}\n', '\n\\end{quote}', '文字'],
          code: ['\\begin{verbatim}\n', '\n\\end{verbatim}', '代码'],
          formula: ['$', '$', 'a+b'],
        }
      : {
          strike: ['~~', '~~', '文字'],
          ordered: ['1. ', '', '列表项'],
          bold: ['**', '**', '文字'],
          italic: ['*', '*', '文字'],
          heading: ['## ', '', '标题'],
          list: ['- ', '', '列表项'],
          quote: ['> ', '', '文字'],
          code: ['`', '`', '代码'],
          formula: ['$', '$', 'a+b'],
        }
    const [before, after, placeholder] = pairs[format]
    if (
      from !== to &&
      from >= before.length &&
      view.state.sliceDoc(from - before.length, from) === before &&
      view.state.sliceDoc(to, to + after.length) === after
    ) {
      view.dispatch({
        changes: [
          { from: from - before.length, to: from },
          { from: to, to: to + after.length },
        ],
        selection: EditorSelection.range(from - before.length, to - before.length),
        userEvent: 'input.format',
      })
      view.focus()
      return true
    }
    const content = view.state.sliceDoc(from, to) || placeholder
    const wrapped =
      content.startsWith(before) &&
      content.endsWith(after) &&
      content.length > before.length + after.length
    const insert = wrapped
      ? content.slice(before.length, after ? -after.length : undefined)
      : before + content + after
    view.dispatch({
      changes: { from, to, insert },
      selection: EditorSelection.range(
        from + (wrapped ? 0 : before.length),
        from + insert.length - (wrapped ? 0 : after.length),
      ),
      userEvent: 'input.format',
    })
  }
  view.focus()
  return true
}

export function statementOutline(content: string, tex = false) {
  const headings: { title: string; level: number; from: number }[] = []
  let offset = 0,
    fence = ''
  for (const line of content.split('\n')) {
    if (!fence && line.trim() === '{{remainingsamples}}')
      headings.push({ title: '样例（来自测试数据）', level: 2, from: offset })
    const marker = !tex && line.match(/^\s{0,3}(`{3,}|~{3,})/)
    if (marker) {
      if (!fence) fence = marker[1]
      else if (marker[1][0] === fence[0] && marker[1].length >= fence.length) fence = ''
    } else if (!fence) {
      const match = tex
        ? line.match(/^\s*\\((?:sub)*)section\*?\{([^}]+)\}/)
        : line.match(/^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/)
      if (match)
        headings.push({
          title: match[2],
          level: tex ? match[1].length / 3 + 1 : match[1].length,
          from: offset,
        })
    }
    offset += line.length + 1
  }
  return headings
}
