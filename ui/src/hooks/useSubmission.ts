import { useCallback, useEffect, useRef, useState } from 'react'
import {
  getApiSubmissionsId as getSubmission,
  getApiSubmissionsIdProgress as getSubmissionProgress,
} from '@/generated/api/vertex'
import type {
  DtoSubmissionProgressResponse as SubmissionProgress,
  DtoSubmissionResponse as Submission,
} from '@/generated/api/model'
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
  const requestController = useRef<AbortController | null>(null)
  requestedId.current = id

  const reload = useCallback(
    async (showSpinner = false) => {
      if (!id) return
      if (showSpinner) setLoading(true)
      requestController.current?.abort()
      const controller = new AbortController()
      requestController.current = controller
      try {
        const result = await getSubmission(id, { signal: controller.signal })
        // A slow response for a previous id must not overwrite the current one.
        if (!controller.signal.aborted && requestedId.current === id) {
          setSubmission(result)
          setError(null)
        }
      } catch (caught) {
        if (!controller.signal.aborted && requestedId.current === id) setError(caught)
      } finally {
        if (requestController.current === controller) requestController.current = null
        if (!controller.signal.aborted && requestedId.current === id) setLoading(false)
      }
    },
    [id],
  )

  const reloadProgress = useCallback(async () => {
    if (!id) return
    requestController.current?.abort()
    const controller = new AbortController()
    requestController.current = controller
    try {
      const progress = await getSubmissionProgress(id, { signal: controller.signal })
      if (controller.signal.aborted || requestedId.current !== id) return
      setSubmission((current) => mergeProgress(current, progress))
      setError(null)
    } catch (caught) {
      if (!controller.signal.aborted && requestedId.current === id) setError(caught)
    } finally {
      if (requestController.current === controller) requestController.current = null
    }
  }, [id])

  useEffect(() => {
    setError(null)
    if (!id) {
      requestController.current?.abort()
      setSubmission(null)
      setLoading(false)
      return
    }

    // Keep the response returned by POST /submissions visible. Clearing it
    // here caused a noticeable flash before the first polling request.
    if (submission?.id === id) {
      setLoading(false)
      return
    }

    setSubmission(null)
    void reload(true)
    return () => requestController.current?.abort()
    // `submission` intentionally does not retrigger the initial load.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, reload])

  const pending = isPendingVerdict(submission?.status)
  useEffect(() => {
    if (!id || !pending) return
    let timer: number | undefined
    let stopped = false

    const poll = async () => {
      if (!document.hidden) await reloadProgress()
      if (!stopped) timer = window.setTimeout(poll, POLL_INTERVAL_MS)
    }

    timer = window.setTimeout(poll, POLL_INTERVAL_MS)
    return () => {
      stopped = true
      requestController.current?.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [id, pending, reloadProgress])

  return { submission, setSubmission, loading, error, reload, pending }
}

function mergeProgress(
  current: Submission | null,
  progress: SubmissionProgress,
): Submission | null {
  if (!current || current.id !== progress.id) return current
  return {
    ...current,
    status: progress.status,
    score: progress.score,
    totalTimeMs: progress.totalTimeMs,
    peakMemoryKb: progress.peakMemoryKb,
    compileResult: progress.compileResult ?? '',
    caseResults: progress.caseResults ?? [],
    judgedCases: progress.judgedCases,
    totalCases: progress.totalCases,
  }
}
