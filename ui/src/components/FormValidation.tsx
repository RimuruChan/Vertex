import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { validationMessage } from '@/lib/validation-message'

type Control = HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement
const isControl = (value: EventTarget | null): value is Control =>
  value instanceof HTMLInputElement ||
  value instanceof HTMLTextAreaElement ||
  value instanceof HTMLSelectElement

/** Replace browser validation balloons without bypassing constraint validation. */
export function FormValidation() {
  const [feedback, setFeedback] = useState<{ target: HTMLElement; message: string } | null>(null)
  const [position, setPosition] = useState({ left: 0, top: 0, visible: false })
  const bubbleRef = useRef<HTMLDivElement>(null)
  const feedbackId = useId()
  useLayoutEffect(() => {
    if (!feedback) return
    const target = feedback.target
    const previous = target.getAttribute('aria-describedby')
    target.setAttribute('aria-describedby', [previous, feedbackId].filter(Boolean).join(' '))
    const update = () => {
      const rect = target.getBoundingClientRect()
      const bubble = bubbleRef.current?.getBoundingClientRect()
      const width = bubble?.width ?? 280
      const height = bubble?.height ?? 44
      setPosition({
        left: Math.max(12, Math.min(rect.left, window.innerWidth - width - 12)),
        top:
          rect.bottom + height + 12 < window.innerHeight
            ? rect.bottom + 8
            : Math.max(12, rect.top - height - 8),
        visible:
          target.isConnected && rect.width > 0 && rect.bottom > 0 && rect.top < window.innerHeight,
      })
    }
    const onFocus = (event: FocusEvent) => {
      if (event.target !== target) setFeedback(null)
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(target)
    if (bubbleRef.current) observer.observe(bubbleRef.current)
    window.addEventListener('scroll', update, true)
    window.addEventListener('resize', update)
    document.addEventListener('focusin', onFocus)
    return () => {
      observer.disconnect()
      window.removeEventListener('scroll', update, true)
      window.removeEventListener('resize', update)
      document.removeEventListener('focusin', onFocus)
      if (previous === null) target.removeAttribute('aria-describedby')
      else target.setAttribute('aria-describedby', previous)
    }
  }, [feedback, feedbackId])
  useEffect(() => {
    const marked = new Map<HTMLElement, string | null>()
    let first: Control | null = null
    let alive = true
    const targetFor = (control: Control): HTMLElement => {
      // Radix's hidden native select performs validation; focus its visible trigger.
      if (control instanceof HTMLSelectElement && control.getAttribute('aria-hidden') === 'true') {
        return (
          [...(control.form?.querySelectorAll<HTMLElement>('[role="combobox"]') ?? [])]
            .filter(
              (trigger) =>
                trigger.compareDocumentPosition(control) & Node.DOCUMENT_POSITION_FOLLOWING,
            )
            .at(-1) ?? control
        )
      }
      return control
    }
    const clear = (target: HTMLElement) => {
      if (!marked.has(target)) return
      const previous = marked.get(target)
      if (previous === null) target.removeAttribute('aria-invalid')
      else if (previous !== undefined) target.setAttribute('aria-invalid', previous)
      target.removeAttribute('data-validation-error')
      marked.delete(target)
    }
    const onInvalid = (event: Event) => {
      if (!isControl(event.target)) return
      event.preventDefault()
      const control = event.target
      const target = targetFor(control)
      if (!marked.has(target)) marked.set(target, target.getAttribute('aria-invalid'))
      target.setAttribute('aria-invalid', 'true')
      target.setAttribute('data-validation-error', '')
      // A submission can invalidate several fields: announce and focus only the first.
      if (first) return
      first = control
      queueMicrotask(() => {
        if (!alive || !first) return
        const field = first
        first = null
        const focusTarget = targetFor(field)
        const label =
          focusTarget.getAttribute('aria-label') || field.labels?.[0]?.textContent?.trim()
        const message =
          field instanceof HTMLSelectElement
            ? '请选择一项'
            : validationMessage({
                validity: field.validity,
                type: field.type,
                minLength: field.minLength,
                maxLength: field.maxLength,
                min: field instanceof HTMLInputElement ? field.min : '',
                max: field instanceof HTMLInputElement ? field.max : '',
                validationMessage: field.validationMessage,
              })
        focusTarget.focus({ preventScroll: true })
        focusTarget.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' })
        setFeedback({ target: focusTarget, message: label ? `${label}：${message}` : message })
      })
    }
    const onEdit = (event: Event) => {
      if (isControl(event.target)) {
        const target = targetFor(event.target)
        if (event.target.validity.valid) clear(target)
        setFeedback((current) => (current?.target === target ? null : current))
      }
    }
    document.addEventListener('invalid', onInvalid, true)
    document.addEventListener('input', onEdit, true)
    document.addEventListener('change', onEdit, true)
    return () => {
      alive = false
      document.removeEventListener('invalid', onInvalid, true)
      document.removeEventListener('input', onEdit, true)
      document.removeEventListener('change', onEdit, true)
      for (const target of marked.keys()) clear(target)
    }
  }, [])
  return feedback
    ? createPortal(
        <div
          ref={bubbleRef}
          id={feedbackId}
          role="alert"
          className="motion-field-error pointer-events-none fixed z-[110] max-w-[min(320px,calc(100vw-24px))] rounded-lg border border-destructive/25 bg-popover px-3 py-2 text-sm text-destructive shadow-md"
          style={{
            left: position.left,
            top: position.top,
            visibility: position.visible ? 'visible' : 'hidden',
          }}
        >
          {feedback.message}
        </div>,
        document.body,
      )
    : null
}
