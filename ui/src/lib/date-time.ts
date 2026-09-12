export function parseLocalDateTime(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const [, y, m, d, h, minute] = match.map(Number)
  const date = new Date(y, m - 1, d, h, minute)
  return date.getFullYear() === y &&
    date.getMonth() === m - 1 &&
    date.getDate() === d &&
    date.getHours() === h &&
    date.getMinutes() === minute
    ? date
    : null
}

export function localDateText(date: Date) {
  return `${date.getFullYear().toString().padStart(4, '0')}-${(date.getMonth() + 1).toString().padStart(2, '0')}-${date.getDate().toString().padStart(2, '0')}`
}

export function calendarDays(month: Date) {
  const first = new Date(month.getFullYear(), month.getMonth(), 1)
  const offset = (first.getDay() + 6) % 7
  return Array.from(
    { length: 42 },
    (_, index) => new Date(first.getFullYear(), first.getMonth(), 1 - offset + index),
  )
}

export function shiftLocalMinutes(value: string, minutes: number) {
  const date = parseLocalDateTime(value)
  if (!date) return ''
  date.setTime(date.getTime() + minutes * 60000)
  return `${localDateText(date)}T${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}
