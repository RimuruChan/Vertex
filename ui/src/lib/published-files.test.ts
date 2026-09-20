import { describe, expect, it } from 'vitest'
import { publishedAssetPath, publishedSamples } from './published-files'
import type { DtoPublishedFileResponse } from '@/generated/api/model'

describe('published statement file projection', () => {
  it('resolves relative image paths without treating remote or query URLs as release assets', () => {
    expect(publishedAssetPath('statement/problem.en.md', '../attachments/diagram.png')).toBe(
      'attachments/diagram.png',
    )
    expect(publishedAssetPath('statement/problem.en.md', 'images/a%20b.png')).toBe(
      'statement/images/a b.png',
    )
    for (const value of [
      'https://other.test/a.png',
      'data:image/png;base64,AA==',
      'image.png?token=secret',
      'image.png#fragment',
    ])
      expect(publishedAssetPath('statement/problem.en.md', value)).toBeUndefined()
  })
  it('orders non-inline samples and keeps binary flags and bounded previews', () => {
    const file = (
      index: number,
      purpose: string,
      extra: Partial<DtoPublishedFileResponse> = {},
    ): DtoPublishedFileResponse => ({
      id: `${index}-${purpose}`,
      path: 'samples/file',
      name: 'file',
      mediaType: 'text/plain',
      purpose,
      size: 100,
      sampleIndex: index,
      ...extra,
    })
    const result = publishedSamples([
      file(2, 'sample-answer'),
      file(1, 'sample-input', { binary: true }),
      file(2, 'sample-input', { truncated: true, preview: 'head' }),
      file(3, 'sample-input', { embedded: true }),
    ])
    expect(result.map((sample) => sample.index)).toEqual([1, 2])
    expect(result[0].input?.binary).toBe(true)
    expect(result[1].input?.preview).toBe('head')
    expect(result[1].answer?.id).toBe('2-sample-answer')
  })
})
