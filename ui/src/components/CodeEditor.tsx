import { useEffect, useRef } from 'react'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState, Compartment } from '@codemirror/state'
import { cpp } from '@codemirror/lang-cpp'
import { python } from '@codemirror/lang-python'
import { oneDark } from '@codemirror/theme-one-dark'
import { useTheme } from '@/components/ThemeProvider'
import { cn } from '@/lib/utils'

function langExtension(language: string) {
  return language === 'python' ? python() : cpp()
}

type CodeEditorProps = {
  value: string
  onChange?: (value: string) => void
  language: string
  /** Read-only mode is used to display an already-submitted source file. */
  readOnly?: boolean
  ariaLabel?: string
  className?: string
}

/**
 * CodeMirror 6 wrapper. The editor fills its container instead of taking a
 * fixed pixel height, so the split-pane layout controls the size.
 */
export default function CodeEditor({
  value,
  onChange,
  language,
  readOnly = false,
  ariaLabel = readOnly ? '只读源代码' : '源代码编辑器',
  className,
}: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  const { resolved } = useTheme()
  const langCompartment = useRef(new Compartment())
  const themeCompartment = useRef(new Compartment())
  const accessCompartment = useRef(new Compartment())

  useEffect(() => {
    if (!containerRef.current) return

    const state = EditorState.create({
      doc: value,
      extensions: [
        basicSetup,
        langCompartment.current.of(langExtension(language)),
        themeCompartment.current.of(resolved === 'dark' ? oneDark : []),
        EditorView.lineWrapping,
        EditorState.tabSize.of(4),
        accessCompartment.current.of([
          EditorState.readOnly.of(readOnly),
          EditorView.editable.of(!readOnly),
          EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
        ]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) onChangeRef.current?.(update.state.doc.toString())
        }),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
    // Initialised once; every input below is swapped through a compartment.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    viewRef.current?.dispatch({
      effects: accessCompartment.current.reconfigure([
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly),
        EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
      ]),
    })
  }, [readOnly, ariaLabel])

  useEffect(() => {
    viewRef.current?.dispatch({
      effects: langCompartment.current.reconfigure(langExtension(language)),
    })
  }, [language])

  useEffect(() => {
    viewRef.current?.dispatch({
      effects: themeCompartment.current.reconfigure(resolved === 'dark' ? oneDark : []),
    })
  }, [resolved])

  // Keeps "reset template" / "clear" / a freshly loaded source in sync.
  useEffect(() => {
    const view = viewRef.current
    if (!view || view.state.doc.toString() === value) return
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
    })
  }, [value])

  return (
    <div
      ref={containerRef}
      className={cn(
        'h-full min-h-0 overflow-hidden rounded-sm border border-border bg-card',
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
