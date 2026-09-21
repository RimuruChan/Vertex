import { useLayoutEffect, useRef, type MutableRefObject } from 'react'
import { EditorView } from '@codemirror/view'
import { EditorState, Compartment, Transaction, Prec } from '@codemirror/state'
import { keymap } from '@codemirror/view'
import { indentWithTab, undo, redo } from '@codemirror/commands'
import { indentUnit, StreamLanguage } from '@codemirror/language'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { stex } from '@codemirror/legacy-modes/mode/stex'
import { openSearchPanel } from '@codemirror/search'
import { formatStatement, type StatementFormat } from '@/lib/statement-editing'
import { cpp } from '@codemirror/lang-cpp'
import { python } from '@codemirror/lang-python'
import { oneDark } from '@codemirror/theme-one-dark'
import { useTheme } from '@/components/ThemeProvider'
import { cn } from '@/lib/utils'
import { editorSetup } from '@/lib/editorSetup'

function langExtension(language: string) {
  if (language === 'markdown' || language === 'tex')
    return [
      language === 'markdown'
        ? markdown({
            base: markdownLanguage,
            codeLanguages: (name) =>
              ['cpp', 'c++', 'c'].includes(name)
                ? cpp().language
                : name === 'python'
                  ? python().language
                  : null,
          })
        : StreamLanguage.define(stex),
      EditorView.lineWrapping,
      Prec.high(
        keymap.of([
          {
            key: 'Mod-b',
            run: (view) =>
              !view.state.readOnly && formatStatement(view, 'bold', language === 'tex'),
          },
          {
            key: 'Mod-i',
            run: (view) =>
              !view.state.readOnly && formatStatement(view, 'italic', language === 'tex'),
          },
        ]),
      ),
    ]
  return language === 'python'
    ? python()
    : ['cpp', 'c'].includes(language)
      ? cpp()
      : [EditorView.lineWrapping]
}

// Use the site's surfaces in both schemes; language highlighting stays separate.
const vertexEditorTheme = EditorView.theme({
  '&': { backgroundColor: 'var(--card)', color: 'var(--foreground)' },
  '.cm-gutters': {
    backgroundColor: 'var(--background)',
    color: 'var(--muted-foreground)',
    border: 'none',
  },
  '.cm-lineNumbers .cm-gutterElement': { padding: '0 8px' },
  '.cm-activeLine, .cm-activeLineGutter': {
    backgroundColor: 'color-mix(in srgb, var(--primary) 7%, var(--card))',
  },
  '.cm-activeLineGutter': { color: 'var(--foreground)' },
  '.cm-content': { caretColor: 'var(--primary)' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--primary)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
    {
      backgroundColor: 'color-mix(in srgb, var(--primary) 24%, transparent)',
    },
  '.cm-panels, .cm-tooltip': {
    backgroundColor: 'var(--popover)',
    color: 'var(--foreground)',
    border: '1px solid var(--border)',
  },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'var(--accent)',
    color: 'var(--accent-foreground)',
  },
})

export type EditorCommands = {
  replace: (content: string) => void
  insert: (before: string, after?: string, placeholder?: string) => void
  format: (format: StatementFormat, tex?: boolean) => void
  undo: () => void
  redo: () => void
  search: () => void
  jump: (from: number) => void
}
type CodeEditorProps = {
  commands?: MutableRefObject<EditorCommands | null>
  value: string
  onChange?: (value: string) => void
  language: string
  /** Read-only mode is used to display an already-submitted source file. */
  readOnly?: boolean
  ariaLabel?: string
  className?: string
  documentKey?: string
}

/**
 * CodeMirror 6 wrapper. The editor fills its container instead of taking a
 * fixed pixel height, so the split-pane layout controls the size.
 */
export default function CodeEditor({
  commands,
  value,
  onChange,
  language,
  readOnly = false,
  ariaLabel = readOnly ? '只读源代码' : '源代码编辑器',
  className,
  documentKey,
}: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  const { resolved } = useTheme()
  const langCompartment = useRef(new Compartment())
  const themeCompartment = useRef(new Compartment())
  const accessCompartment = useRef(new Compartment())
  const currentDocument = useRef(documentKey)
  const documents = useRef(
    new Map<
      string | undefined,
      { state: EditorState; scroll: ReturnType<EditorView['scrollSnapshot']> }
    >(),
  )

  function createState() {
    return EditorState.create({
      doc: value,
      extensions: [
        editorSetup,
        vertexEditorTheme,
        langCompartment.current.of(langExtension(language)),
        themeCompartment.current.of(resolved === 'dark' ? oneDark : []),
        EditorState.tabSize.of(4),
        indentUnit.of('    '),
        accessCompartment.current.of([
          EditorState.readOnly.of(readOnly),
          EditorView.editable.of(!readOnly),
          keymap.of(readOnly ? [] : [indentWithTab]),
          EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
        ]),
        EditorView.updateListener.of((update) => {
          if (
            update.docChanged &&
            !update.transactions.some((tr) => tr.annotation(Transaction.remote))
          ) {
            onChangeRef.current?.(update.state.doc.toString())
          }
        }),
      ],
    })
  }
  const createStateRef = useRef(createState)
  createStateRef.current = createState

  useLayoutEffect(() => {
    if (!containerRef.current) return
    const view = new EditorView({ state: createStateRef.current(), parent: containerRef.current })
    viewRef.current = view
    if (commands)
      commands.current = {
        replace(content) {
          if (view.state.readOnly) return
          const { from, to } = view.state.selection.main
          view.dispatch({
            changes: { from, to, insert: content },
            selection: { anchor: from + content.length },
            userEvent: 'input',
          })
          view.focus()
        },
        format(format, tex) {
          if (!view.state.readOnly) formatStatement(view, format, tex)
        },
        undo() {
          if (!view.state.readOnly) undo(view)
          view.focus()
        },
        redo() {
          if (!view.state.readOnly) redo(view)
          view.focus()
        },
        search() {
          openSearchPanel(view)
        },
        jump(from) {
          view.dispatch({
            selection: { anchor: Math.min(from, view.state.doc.length) },
            effects: EditorView.scrollIntoView(Math.min(from, view.state.doc.length), {
              y: 'start',
            }),
          })
          view.focus()
        },
        insert(before, after = '', placeholder = '') {
          if (view.state.facet(EditorState.readOnly)) return
          const { from, to } = view.state.selection.main
          const content = view.state.sliceDoc(from, to) || placeholder
          view.dispatch({
            changes: { from, to, insert: before + content + after },
            selection: {
              anchor: from + before.length,
              head: from + before.length + content.length,
            },
          })
          view.focus()
        },
      }
    return () => {
      if (commands) commands.current = null
      view.destroy()
      viewRef.current = null
    }
    // Initialised once; every input below is swapped through a compartment.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useLayoutEffect(() => {
    const view = viewRef.current
    if (!view || currentDocument.current === documentKey) return
    documents.current.set(currentDocument.current, {
      state: view.state,
      scroll: view.scrollSnapshot(),
    })
    const cached = documents.current.get(documentKey)
    currentDocument.current = documentKey
    if (cached && cached.state.doc.toString() === value) {
      view.setState(cached.state)
      view.dispatch({ effects: cached.scroll })
    } else {
      view.setState(createStateRef.current())
    }
    // Bound memory while keeping recent problems/languages convenient to revisit.
    documents.current.delete(documentKey)
    while (documents.current.size > 12)
      documents.current.delete(documents.current.keys().next().value)
  }, [documentKey, value])

  useLayoutEffect(() => {
    viewRef.current?.dispatch({
      effects: accessCompartment.current.reconfigure([
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly),
        keymap.of(readOnly ? [] : [indentWithTab]),
        EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
      ]),
    })
  }, [readOnly, ariaLabel, documentKey])

  useLayoutEffect(() => {
    viewRef.current?.dispatch({
      effects: langCompartment.current.reconfigure(langExtension(language)),
    })
  }, [language, documentKey])

  useLayoutEffect(() => {
    viewRef.current?.dispatch({
      effects: themeCompartment.current.reconfigure(resolved === 'dark' ? oneDark : []),
    })
  }, [resolved, documentKey])

  // Keeps "reset template" / "clear" / a freshly loaded source in sync.
  useLayoutEffect(() => {
    const view = viewRef.current
    if (!view || view.state.doc.toString() === value) return
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
      annotations: Transaction.remote.of(true),
    })
  }, [value, documentKey])

  return (
    <div
      ref={containerRef}
      className={cn(
        'h-full min-h-0 overflow-hidden rounded-sm border border-border bg-card focus-within:border-ring',
        '[&_.cm-editor]:h-full [&_.cm-scroller]:overflow-auto',
        className,
      )}
    />
  )
}

/** Starter code offered when a language is selected. */
export const languageTemplates: Record<string, string> = {
  cpp: `#include <bits/stdc++.h>
using namespace std;

int main() {
    ios::sync_with_stdio(false);
    cin.tie(nullptr);

    // TODO: 读取输入,输出答案
    return 0;
}
`,
  c: `#include <stdio.h>

int main() {
    // TODO: 读取输入,输出答案
    return 0;
}
`,
  python: `import sys

def main():
    data = sys.stdin.read().split()
    # TODO: 读取输入,输出答案

if __name__ == "__main__":
    main()
`,
}

export const languageOptions = [
  { value: 'cpp', label: 'C++17' },
  { value: 'c', label: 'C11' },
  { value: 'python', label: 'Python 3' },
]
