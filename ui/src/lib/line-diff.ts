export type DiffLine = {
  kind: 'same' | 'removed' | 'added'
  text: string
  before?: number
  after?: number
}

/** Bounded line comparison for review, never used to apply or resolve a merge. */
export function lineDiff(before: string, after: string) {
  const maxChars = 128000,
    maxLines = 3000
  const split = (text: string) =>
    text ? text.slice(0, maxChars).split('\n').slice(0, maxLines) : []
  const a = split(before),
    b = split(after)
  const truncated =
    before.length > maxChars ||
    after.length > maxChars ||
    a.join('\n') !== before ||
    b.join('\n') !== after
  const lines: DiffLine[] = []
  let prefix = 0,
    suffix = 0
  while (prefix < a.length && prefix < b.length && a[prefix] === b[prefix]) prefix++
  while (
    suffix < a.length - prefix &&
    suffix < b.length - prefix &&
    a[a.length - 1 - suffix] === b[b.length - 1 - suffix]
  )
    suffix++
  const same = (i: number, j: number) =>
    lines.push({ kind: 'same', text: a[i], before: i + 1, after: j + 1 })
  for (let i = 0; i < prefix; i++) same(i, i)
  const n = a.length - prefix - suffix,
    m = b.length - prefix - suffix
  const coarse = (n + 1) * (m + 1) > 750000
  let i = 0,
    j = 0
  if (coarse) {
    for (; i < n; i++) lines.push({ kind: 'removed', text: a[prefix + i], before: prefix + i + 1 })
    for (; j < m; j++) lines.push({ kind: 'added', text: b[prefix + j], after: prefix + j + 1 })
  } else {
    const width = m + 1,
      lengths = new Uint16Array((n + 1) * width)
    for (let x = n - 1; x >= 0; x--)
      for (let y = m - 1; y >= 0; y--)
        lengths[x * width + y] =
          a[prefix + x] === b[prefix + y]
            ? lengths[(x + 1) * width + y + 1] + 1
            : Math.max(lengths[(x + 1) * width + y], lengths[x * width + y + 1])
    while (i < n || j < m) {
      if (i < n && j < m && a[prefix + i] === b[prefix + j]) {
        same(prefix + i, prefix + j)
        i++
        j++
        continue
      }
      if (i < n && (j === m || lengths[(i + 1) * width + j] >= lengths[i * width + j + 1])) {
        lines.push({ kind: 'removed', text: a[prefix + i], before: prefix + i + 1 })
        i++
      } else {
        lines.push({ kind: 'added', text: b[prefix + j], after: prefix + j + 1 })
        j++
      }
    }
  }
  for (let k = suffix; k > 0; k--) same(a.length - k, b.length - k)
  return { lines, truncated, coarse }
}
