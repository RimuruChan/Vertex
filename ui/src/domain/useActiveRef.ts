import { useEffect, useRef } from 'react'

export function useActiveRef() {
  const active = useRef(true)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  return active
}
