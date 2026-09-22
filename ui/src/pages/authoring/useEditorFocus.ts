import { useLayoutEffect, type RefObject } from 'react'

/** Keep the existing editor mounted while temporarily isolating its workspace. */
export function useEditorFocus(
  root: RefObject<HTMLDivElement | null>,
  focused: boolean,
  onExit: () => void,
) {
  useLayoutEffect(() => {
    const editor = root.current
    if (!focused || !editor) return
    const active = document.activeElement
    const hidden: { element: HTMLElement; inert: boolean }[] = []
    let branch: HTMLElement = editor
    while (branch.parentElement) {
      for (const sibling of branch.parentElement.children) {
        if (sibling === branch || !(sibling instanceof HTMLElement)) continue
        hidden.push({ element: sibling, inert: sibling.inert })
        sibling.inert = true
      }
      if (branch.parentElement === document.body) break
      branch = branch.parentElement
    }
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const frame = requestAnimationFrame(() => {
      editor.querySelector<HTMLElement>('[aria-label="题面内容"]')?.focus()
    })
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || event.isComposing) return
      // Let an open menu, dialog or CodeMirror search consume Escape first.
      if (document.querySelector('[role="dialog"], [role="alertdialog"], [role="menu"]')) return
      if (event.target instanceof Element && event.target.closest('.cm-search')) return
      event.preventDefault()
      onExit()
    }
    window.addEventListener('keydown', escape)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('keydown', escape)
      document.body.style.overflow = overflow
      hidden.forEach(({ element, inert }) => {
        element.inert = inert
      })
      if (active instanceof HTMLElement && active.isConnected) active.focus({ preventScroll: true })
    }
  }, [root, focused, onExit])
}
