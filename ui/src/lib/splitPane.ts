export const DIVIDER_WIDTH = 12
export const MIN_PANE_WIDTH = 320

export function splitLimits(width: number) {
  const available = Math.max(1, width - DIVIDER_WIDTH)
  const min = Math.min(50, Math.max(25, (MIN_PANE_WIDTH / available) * 100))
  return { min, max: 100 - min }
}

export function pointerSplit(offset: number, width: number) {
  const { min, max } = splitLimits(width)
  if (Math.abs(offset - width / 2) <= 24) return 50
  const percent = ((offset - DIVIDER_WIDTH / 2) / Math.max(1, width - DIVIDER_WIDTH)) * 100
  return Math.min(max, Math.max(min, percent))
}
