import type { DomainTreeEntry } from '@/generated/api/model'

/** Scoped to one problem workspace. Opening a program never fetches its helpers. */
export function programSourceReader(fetch: (digest: string) => Promise<Blob>) {
  const cache = new Map<string, { bytes: number; value: Promise<string | undefined> }>()
  let bytes = 0
  return (entry: DomainTreeEntry): Promise<string | undefined> => {
    if (!entry.blob.sha256 || entry.blob.bytes > 1024 * 1024) return Promise.resolve(undefined)
    const key = entry.blob.sha256,
      cached = cache.get(key)
    if (cached) return cached.value
    while (cache.size && (cache.size >= 32 || bytes + entry.blob.bytes > 8 * 1024 * 1024)) {
      const oldest = cache.keys().next().value!
      bytes -= cache.get(oldest)!.bytes
      cache.delete(oldest)
    }
    const value = fetch(key)
      .then(async (blob) => {
        if (blob.size > 1024 * 1024) return undefined
        try {
          const text = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
          return text.includes('\0') ? undefined : text
        } catch {
          return undefined
        }
      })
      .catch((error) => {
        if (cache.get(key)?.value === value) {
          bytes -= cache.get(key)!.bytes
          cache.delete(key)
        }
        throw error
      })
    cache.set(key, { bytes: entry.blob.bytes, value })
    bytes += entry.blob.bytes
    return value
  }
}
