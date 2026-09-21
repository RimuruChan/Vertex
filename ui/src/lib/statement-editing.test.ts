import { expect, it } from 'vitest'
import { EditorState } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { StreamLanguage } from '@codemirror/language'
import { stex } from '@codemirror/legacy-modes/mode/stex'
import { formatStatement, statementOutline } from './statement-editing'

function editor(doc: string, from: number, to = from) {
  const view = {
    state: EditorState.create({ doc, selection: { anchor: from, head: to } }),
    dispatch(spec: Parameters<EditorState['update']>[0]) {
      this.state = this.state.update(spec).state
    },
    focus() {},
  }
  return view as unknown as EditorView
}
it('formats whole lines without injecting a heading into the middle of a sentence', () => {
  const view = editor('前言\n题目描述\n正文', 6)
  formatStatement(view, 'heading')
  expect(view.state.doc.toString()).toBe('前言\n## 题目描述\n正文')
  formatStatement(view, 'heading')
  expect(view.state.doc.toString()).toBe('前言\n题目描述\n正文')
})
it('formats multiline lists without consuming the following unselected line', () => {
  const view = editor('a\nb\nc', 0, 4)
  formatStatement(view, 'list')
  expect(view.state.doc.toString()).toBe('- a\n- b\nc')
})
it('changes heading levels without stacking markers and recognises nested TeX sections', () => {
  const view = editor('## 描述\n正文', 4)
  formatStatement(view, 'heading', false, 6)
  expect(view.state.doc.toString()).toBe('###### 描述\n正文')
  formatStatement(view, 'heading', false, 1)
  expect(view.state.doc.toString()).toBe('# 描述\n正文')
  const tex = editor('细节', 0, 2)
  formatStatement(tex, 'heading', true, 3)
  expect(tex.state.doc.toString()).toBe('\\subsubsection{细节}')
  expect(statementOutline(tex.state.doc.toString(), true)).toEqual([
    { title: '细节', level: 3, from: 0 },
  ])
})
it('preserves TeX source around an inline formatting edit', () => {
  const view = editor('before 演示 after', 7, 9)
  formatStatement(view, 'bold', true)
  expect(view.state.doc.toString()).toBe('before \\textbf{演示} after')
  formatStatement(view, 'bold', true)
  expect(view.state.doc.toString()).toBe('before 演示 after')
})
it('ignores headings inside fenced code and exposes real document offsets', () => {
  const doc = '## 描述\n```md\n# not a heading\n```\n## 输入'
  expect(statementOutline(doc)).toEqual([
    { title: '描述', level: 2, from: 0 },
    { title: '输入', level: 2, from: doc.indexOf('## 输入') },
  ])
})
it('loads both real editor language extensions, including Markdown embedded grammars', () => {
  expect(() =>
    EditorState.create({
      doc: '# Hello\n\n```cpp\nint x;\n```',
      extensions: markdown({ base: markdownLanguage }),
    }),
  ).not.toThrow()
  expect(() =>
    EditorState.create({ doc: '\\section{Hello}', extensions: StreamLanguage.define(stex) }),
  ).not.toThrow()
})
