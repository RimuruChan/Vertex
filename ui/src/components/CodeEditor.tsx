import { useEffect, useRef } from 'react'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState, Compartment } from '@codemirror/state'
import { cpp } from '@codemirror/lang-cpp'
import { python } from '@codemirror/lang-python'
import { java } from '@codemirror/lang-java'
import { oneDark } from '@codemirror/theme-one-dark'

// CodeEditor:基于 CodeMirror 6 的轻量代码编辑器(语法高亮,无重量级依赖)。
// 判题页只需要高亮 + 基础编辑,不需要 Monaco 的完整能力。
export default function CodeEditor({
  value,
  onChange,
  language,
  height = 320,
}: {
  value: string
  onChange: (v: string) => void
  language: string
  height?: number
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  // 语言扩展用 Compartment 管理,切换语言时 reconfigure
  const langCompartmentRef = useRef(new Compartment())

  function langExtension(lang: string) {
    return lang === 'python' ? python() : lang === 'java' ? java() : cpp()
  }

  useEffect(() => {
    if (!containerRef.current) return

    const langCompartment = langCompartmentRef.current

    const state = EditorState.create({
      doc: value,
      extensions: [
        basicSetup,
        langCompartment.of(langExtension(language)),
        oneDark,
        EditorView.lineWrapping,
        EditorState.tabSize.of(4),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current(update.state.doc.toString())
          }
        }),
        EditorView.theme({
          '&': { fontSize: '13px', height: `${height}px` },
          '.cm-scroller': { overflow: 'auto' },
        }),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
  }, []) // 仅首次初始化

  // 语言变化时仅切换语言扩展
  useEffect(() => {
    if (viewRef.current) {
      viewRef.current.dispatch({
        effects: langCompartmentRef.current.reconfigure(langExtension(language)),
      })
    }
  }, [language])

  // 同步“重置模板/清空”等外部受控值变化。
  useEffect(() => {
    const view = viewRef.current
    if (!view || view.state.doc.toString() === value) return
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
  }, [value])

  return (
    <div
      ref={containerRef}
      style={{
        border: '1px solid #d9d9d9',
        borderRadius: 6,
        overflow: 'hidden',
        background: '#282c34',
      }}
    />
  )
}

// 各语言的代码模板
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
