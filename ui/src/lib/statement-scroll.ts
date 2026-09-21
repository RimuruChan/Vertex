import { lineDiff } from './line-diff'

/** Preview expands sample placeholders. Keep unchanged lines anchored to the source,
 * and associate inserted sample blocks with the placeholder they replaced. */
export function previewSourceLines(source: string, preview: string): number[] {
  if (source === preview) return preview.split('\n').map((_, i) => i)
  const diff = lineDiff(source, preview)
  const last = Math.max(0, source.split('\n').length - 1)
  const result: number[] = []
  let cursor = 0
  let replacement: number | undefined
  for (const line of diff.lines) {
    if (line.kind === 'same') {
      result[line.after! - 1] = line.before! - 1
      cursor = line.before!
      // Sample expansion may keep a blank line before inserting the generated
      // block. Such whitespace must not detach it from its removed placeholder.
      if (line.text.trim()) replacement = undefined
    } else if (line.kind === 'removed') {
      replacement ??= line.before! - 1
      cursor = line.before!
    } else {
      result[line.after! - 1] = Math.min(last, replacement ?? cursor)
    }
  }
  // The bounded diff can omit the end of an unusually large statement. Do not
  // invent paragraph anchors there; the scroll map still has an end anchor.
  return result
}

export type ScrollAnchor = { source: number; preview: number }
export function scrollAnchors(points: ScrollAnchor[], sourceMax: number, previewMax: number) {
  const anchors: ScrollAnchor[] = [{ source: 0, preview: 0 }]
  for (const point of points) {
    const previous = anchors[anchors.length - 1]
    if (
      point.source > previous.source &&
      point.preview > previous.preview &&
      point.source < sourceMax &&
      point.preview < previewMax
    )
      anchors.push(point)
  }
  anchors.push({ source: Math.max(0, sourceMax), preview: Math.max(0, previewMax) })
  return anchors
}

export function linkedScrollTop(anchors: ScrollAnchor[], from: keyof ScrollAnchor, top: number) {
  const to = from === 'source' ? 'preview' : 'source'
  for (let i = 1; i < anchors.length; i++) {
    const a = anchors[i - 1],
      b = anchors[i]
    if (top <= b[from]) {
      const fraction = b[from] > a[from] ? Math.max(0, (top - a[from]) / (b[from] - a[from])) : 0
      return a[to] + fraction * (b[to] - a[to])
    }
  }
  return anchors.at(-1)?.[to] ?? 0
}
