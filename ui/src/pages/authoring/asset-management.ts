import type { DomainTreeEntry } from '@/generated/api/model'

export type AssetType = 'all' | 'image' | 'document' | 'archive' | 'other'

export function assetScope(entry: DomainTreeEntry): 'public' | 'private' {
  return entry.kind === 'asset' && entry.attributes.visibility !== 'private' ? 'public' : 'private'
}

export function assetType(path: string): Exclude<AssetType, 'all'> {
  if (/\.(png|jpe?g|webp|gif|svg|avif|bmp|ico)$/i.test(path)) return 'image'
  if (/\.(txt|md|tex|pdf|json|ya?ml|csv|tsv|cpp|py|c|h|hpp|java|js|ts|html|css|xml)$/i.test(path))
    return 'document'
  if (/\.(zip|tar|gz|bz2|xz|7z|rar)$/i.test(path)) return 'archive'
  return 'other'
}

export function assetUploadErrors(files: File[]): string[] {
  const errors: string[] = []
  if (files.length > 100) errors.push('每次最多上传 100 个文件')
  for (const file of files) {
    if (!file.name.trim()) errors.push('文件缺少名称')
    if (file.size > 64 * 1024 * 1024) errors.push(`${file.name} 超过 64 MiB`)
  }
  return errors
}

export function assetPreviewKind(entry: DomainTreeEntry): 'image' | 'text' | 'unavailable' {
  if (/\.(png|jpe?g|webp|gif)$/i.test(entry.path) && entry.blob.bytes <= 8 * 1024 * 1024)
    return 'image'
  if (
    /\.(txt|md|tex|json|ya?ml|csv|tsv|cpp|py|c|h|hpp|java|js|ts|html|css|xml)$/i.test(entry.path) &&
    entry.blob.bytes < 64 * 1024
  )
    return 'text'
  return 'unavailable'
}
