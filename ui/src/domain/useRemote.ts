import { useEffect, useState } from 'react'
import { apiError } from '@/lib/format'

// Callers memoize load with the resource identity; each read can be aborted.
export function useRemote<T>(load: (signal: AbortSignal) => Promise<T>) {
  const [data, setData] = useState<T | null>(null),
    [error, setError] = useState<string | null>(null),
    [loading, setLoading] = useState(true),
    [version, setVersion] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    load(controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setData(value)
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(apiError(cause, '加载失败'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [load, version])
  return { data, error, loading, reload: () => setVersion((value) => value + 1) }
}
