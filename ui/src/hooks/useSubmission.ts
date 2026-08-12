import { useCallback, useEffect, useRef, useState } from 'react'
import { getApiSubmissionsId as getSubmission } from '@/generated/api/vertex'
import type { DtoSubmissionResponse as Submission } from '@/generated/api/model'
import { isPendingVerdict } from '@/components/VerdictTag'

const POLL_INTERVAL_MS = 1500

/**
 * Loads one submission and keeps polling while the judge is still working on
 * it, so the verdict and per-case progress arrive without a manual refresh.
 * Polling stops as soon as the submission reaches a terminal verdict.
 */
export function useSubmission(id: string | undefined) {
  const [submission, setSubmission] = useState<Submission | null>(null)
  const [loading, setLoading] = useState(Boolean(id))
  const [error, setError] = useState<unknown>(null)
  const requestedId = useRef(id)
  requestedId.current = id

  const reload = useCallback(async () => {
    if (!id) return
    try {
      const result = await getSubmission(id)
      // A slow response for a previous id must not overwrite the current one.
      if (requestedId.current === id) {
        setSubmission(result)
        setError(null)
      }
    } catch (caught) {
      if (requestedId.current === id) setError(caught)
    } finally {
      if (requestedId.current === id) setLoading(false)
    }
  }, [id])

  useEffect(() => {
    setSubmission(null)
    setError(null)
    setLoading(Boolean(id))
    void reload()
  }, [id, reload])

  const pending = isPendingVerdict(submission?.status)
  useEffect(() => {
    if (!id || !pending) return
    const timer = window.setInterval(() => void reload(), POLL_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [id, pending, reload])

  return { submission, setSubmission, loading, error, reload, pending }
}
