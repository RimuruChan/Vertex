import { useEffect, type RefObject } from 'react'
import type { EditorCommands } from '@/components/CodeEditor'
import { linkedScrollTop, scrollAnchors, type ScrollAnchor } from '@/lib/statement-scroll'

export function useStatementScrollSync(
  commands: RefObject<EditorCommands | null>,
  previewRef: RefObject<HTMLElement | null>,
  enabled: boolean,
  document: string,
) {
  useEffect(() => {
    const editor = commands.current
    const preview = previewRef.current
    if (!enabled || !editor || !preview) return
    const source = editor.scrollElement()
    const panes = { source, preview }
    const expected: Partial<Record<keyof ScrollAnchor, number>> = {}
    const previous = { source: source.scrollTop, preview: preview.scrollTop }
    let leader: keyof ScrollAnchor = 'source'
    let frame = 0
    const sync = () => {
      frame = 0
      const previewTop = preview.getBoundingClientRect().top
      const points = [...preview.querySelectorAll<HTMLElement>('[data-source-line]')].map(
        (node) => ({
          source: editor.lineTop(Number(node.dataset.sourceLine)),
          preview: node.getBoundingClientRect().top - previewTop + preview.scrollTop,
        }),
      )
      const anchors = scrollAnchors(
        points,
        source.scrollHeight - source.clientHeight,
        preview.scrollHeight - preview.clientHeight,
      )
      const follower = leader === 'source' ? 'preview' : 'source'
      const target = panes[follower]
      const top = linkedScrollTop(anchors, leader, panes[leader].scrollTop)
      if (Math.abs(target.scrollTop - top) < 1) return
      target.scrollTop = top
      // Compare the browser's clamped value. Ignore only our own resulting event,
      // so an immediate wheel/touch/keyboard scroll on the other pane still wins.
      expected[follower] = target.scrollTop
    }
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(sync)
    }
    const reflow = () => {
      // Replacing the sample preview or resizing a pane can clamp its scrollTop.
      // That is a layout change, not a request to drive the other pane.
      expected.source = source.scrollTop
      expected.preview = preview.scrollTop
      schedule()
    }
    const scrolled = (side: keyof ScrollAnchor) => {
      const top = panes[side].scrollTop
      if (expected[side] !== undefined && Math.abs(top - expected[side]!) < 1) {
        delete expected[side]
        previous[side] = top
        return
      }
      delete expected[side]
      if (Math.abs(previous[side] - top) < 0.5) return
      previous[side] = top
      leader = side
      schedule()
    }
    const onSource = () => scrolled('source'),
      onPreview = () => scrolled('preview')
    source.addEventListener('scroll', onSource, { passive: true })
    preview.addEventListener('scroll', onPreview, { passive: true })
    preview.addEventListener('load', reflow, true)
    const resize = new ResizeObserver(reflow)
    resize.observe(source)
    resize.observe(preview)
    if (preview.firstElementChild) resize.observe(preview.firstElementChild)
    const changes = new MutationObserver(reflow)
    changes.observe(preview, { childList: true, subtree: true })
    schedule()
    return () => {
      cancelAnimationFrame(frame)
      source.removeEventListener('scroll', onSource)
      preview.removeEventListener('scroll', onPreview)
      preview.removeEventListener('load', reflow, true)
      resize.disconnect()
      changes.disconnect()
    }
  }, [commands, previewRef, enabled, document])
}
