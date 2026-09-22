import { describe, expect, it } from 'vitest'
import { assetScope, assetType, assetUploadErrors, assetPreviewKind } from './asset-management'
import type { DomainTreeEntry } from '@/generated/api/model'

const entry: DomainTreeEntry = {
  id: 'asset',
  kind: 'asset',
  path: 'attachments/picture.png',
  attributes: {},
  blob: { sha256: '', bytes: 0 },
}

describe('asset management', () => {
  it('keeps private assets and resource entries out of the public scope', () => {
    expect(assetScope(entry)).toBe('public')
    expect(assetScope({ ...entry, attributes: { visibility: 'private' } })).toBe('private')
    expect(assetScope({ ...entry, kind: 'resource', attributes: { visibility: 'public' } })).toBe(
      'private',
    )
  })
  it('classifies image, document and archive names without case sensitivity', () => {
    expect(assetType('attachments/PHOTO.PNG')).toBe('image')
    expect(assetType('resources/reference.PDF')).toBe('document')
    expect(assetType('resources/data.tar.gz')).toBe('archive')
    expect(assetType('resources/file')).toBe('other')
  })
  it('finds invalid later files before a batch starts uploading', () => {
    const huge = new File([''], 'too-large.zip')
    Object.defineProperty(huge, 'size', { value: 64 * 1024 * 1024 + 1 })
    expect(assetUploadErrors([new File(['ok'], 'valid.txt'), huge])).toEqual([
      'too-large.zip 超过 64 MiB',
    ])
    expect(assetUploadErrors([new File([''], 'empty.txt')])).toEqual([])
  })
  it('allows exactly 100 files and rejects a larger batch', () => {
    const files = Array.from({ length: 100 }, (_, i) => new File([''], `${i}.txt`))
    expect(assetUploadErrors(files)).toEqual([])
    expect(assetUploadErrors([...files, new File([''], 'extra.txt')])).toContain(
      '每次最多上传 100 个文件',
    )
  })
  it('does not treat a large or unsupported image as a pending preview', () => {
    expect(assetPreviewKind(entry)).toBe('image')
    expect(assetPreviewKind({ ...entry, blob: { sha256: '', bytes: 8 * 1024 * 1024 + 1 } })).toBe(
      'unavailable',
    )
    expect(assetPreviewKind({ ...entry, path: 'attachments/vector.svg' })).toBe('unavailable')
    expect(assetPreviewKind({ ...entry, path: 'resources/empty.txt' })).toBe('text')
  })
})
