import type { DtoPublishedFileResponse } from '@/generated/api/model'

export function publishedAssetPath(statementPath: string, destination: string): string | undefined {
  try {
    const base = new URL(statementPath, 'https://vertex.invalid/')
    const resolved = new URL(destination, base)
    if (resolved.origin !== base.origin || resolved.search || resolved.hash) return undefined
    return decodeURIComponent(resolved.pathname.slice(1))
  } catch {
    return undefined
  }
}
export function publishedSamples(files: DtoPublishedFileResponse[]) {
  const indexes = [
    ...new Set(
      files.filter((file) => file.sampleIndex && !file.embedded).map((file) => file.sampleIndex!),
    ),
  ].sort((a, b) => a - b)
  return indexes.map((index) => ({
    index,
    input: files.find((file) => file.sampleIndex === index && file.purpose === 'sample-input'),
    answer: files.find((file) => file.sampleIndex === index && file.purpose === 'sample-answer'),
  }))
}
