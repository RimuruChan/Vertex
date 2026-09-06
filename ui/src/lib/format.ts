/** Shared display helpers so units and wording stay identical across pages. */

export function formatMemory(kb: number): string {
  if (!kb) return '0 MB'
  return `${(kb / 1024).toFixed(kb < 10240 ? 1 : 0)} MB`
}

export function formatTime(ms: number): string {
  return `${ms} ms`
}

export function formatRatio(accepted: number, total: number): string {
  if (total <= 0) return '—'
  return `${((accepted / total) * 100).toFixed(1)}%`
}

export function formatDateTime(value: string | Date): string {
  return new Date(value).toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatDate(value: string | Date): string {
  return new Date(value).toLocaleDateString()
}

/** Coarse relative time; exact timestamps stay available in tooltips. */
export function formatRelative(value: string | Date): string {
  const target = new Date(value).getTime()
  const seconds = Math.round((target - Date.now()) / 1000)
  const absoluteSeconds = Math.abs(seconds)
  const direction = seconds > 0 ? '后' : '前'
  if (absoluteSeconds < 5) return '刚刚'
  if (absoluteSeconds < 60) return `不到 1 分钟${direction}`
  if (absoluteSeconds < 3600) return `${Math.floor(absoluteSeconds / 60)} 分钟${direction}`
  if (absoluteSeconds < 86400) return `${Math.floor(absoluteSeconds / 3600)} 小时${direction}`
  if (absoluteSeconds < 2592000) return `${Math.floor(absoluteSeconds / 86400)} 天${direction}`
  return formatDate(value)
}

/**
 * `<input type="datetime-local">` speaks local wall-clock time with no zone,
 * while the API speaks ISO-8601 UTC. These two convert between them.
 */
export function toLocalInput(value: string | undefined): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

export function fromLocalInput(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
}

/** Short id prefix used wherever a UUID has to be shown to a human. */
export function shortId(id: string): string {
  return id.slice(0, 8)
}

export function apiError(error: unknown, fallback: string): string {
  const response = (error as { response?: { data?: { error?: string } } })?.response
  return response?.data?.error ?? fallback
}

/** Difficulty 1-10 mapped to the labels shown next to problems. */
export function difficultyLabel(difficulty: number): { label: string; className: string } {
  if (difficulty <= 3) return { label: '入门', className: 'bg-verdict-ac-bg text-verdict-ac' }
  if (difficulty <= 6) return { label: '进阶', className: 'bg-verdict-tle-bg text-verdict-tle' }
  if (difficulty <= 8) return { label: '困难', className: 'bg-verdict-wa-bg text-verdict-wa' }
  return { label: '极难', className: 'bg-verdict-err-bg text-verdict-err' }
}

export const languageLabels: Record<string, string> = {
  cpp: 'C++17',
  c: 'C11',
  python: 'Python 3',
}

export function languageLabel(language: string): string {
  return languageLabels[language] ?? language
}
