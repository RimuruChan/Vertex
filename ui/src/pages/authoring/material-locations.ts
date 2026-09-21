import type { DomainTreeEntry } from '@/generated/api/model'

/** Allocation is internal; imported locations are preserved, never normalized in place. */
export function availableLocation(
  preferred: string,
  entries: Pick<DomainTreeEntry, 'path'>[],
  id: string,
) {
  const paths = entries.map((entry) => entry.path.toLowerCase())
  const conflicts = (candidate: string) =>
    paths.some(
      (path) =>
        path === candidate.toLowerCase() ||
        path.startsWith(candidate.toLowerCase() + '/') ||
        candidate.toLowerCase().startsWith(path + '/'),
    )
  if (!conflicts(preferred)) return preferred
  const slash = preferred.lastIndexOf('/')
  const directory = preferred.slice(0, slash + 1),
    name = preferred.slice(slash + 1)
  let candidate = `${directory}${id}-${name}`
  // An imported file may shadow even the managed directory itself.
  if (conflicts(candidate)) candidate = `material-${id}/${name}`
  for (let n = 2; conflicts(candidate); n++) candidate = `material-${id}-${n}/${name}`
  return candidate
}

export function programDirectory(files: string[], entries: DomainTreeEntry[], previous = '') {
  const paths = files.flatMap((id) => {
    const e = entries.find((entry) => entry.id === id)
    return e ? [e.path] : []
  })
  // Retain an imported root when it still contains every selected file.
  if (previous && paths.every((path) => path.startsWith(previous + '/'))) return previous
  if (!paths.length) return ''
  const parts = paths[0].split('/').slice(0, -1)
  while (parts.length && !paths.every((path) => path.startsWith(parts.join('/') + '/'))) parts.pop()
  return parts.join('/')
}
