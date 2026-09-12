import { useEffect } from 'react'

/** Keep native scrolling and dragging, showing each thumb only while in use. */
export function Scrollbars() {
  useEffect(() => {
    const timers = new Map<Element, number>()
    const onScroll = (event: Event) => {
      const element = event.target === document ? document.documentElement : event.target
      if (!(element instanceof Element)) return
      window.clearTimeout(timers.get(element))
      element.setAttribute('data-scrolling', '')
      timers.set(
        element,
        window.setTimeout(() => {
          element.removeAttribute('data-scrolling')
          timers.delete(element)
        }, 900),
      )
    }

    document.addEventListener('scroll', onScroll, { capture: true, passive: true })
    return () => {
      document.removeEventListener('scroll', onScroll, true)
      for (const [element, timer] of timers) {
        window.clearTimeout(timer)
        element.removeAttribute('data-scrolling')
      }
    }
  }, [])

  return null
}
