import { expect, it } from 'vitest'
import { metadataError } from './editor-validation'
import { defaultMetadata } from '@/lib/authoring-materials'
import { attachmentMarkdown } from './asset-links'
import type { DomainTreeEntry } from '@/generated/api/model'

it('does not autosave an incomplete numeric field or empty title', () => {
  const value = defaultMetadata('Sum')
  expect(metadataError(JSON.stringify(value))).toBe('')
  for (const patch of [
    { title: '' },
    { timeLimitMs: 0 },
    { memoryLimitKb: 0 },
    { timeLimitMs: 1.5 },
  ])
    expect(metadataError(JSON.stringify({ ...value, ...patch }))).not.toBe('')
})
it('inserts portable attachment links without treating names as markup', () => {
  const entry = (path: string, format = ''): DomainTreeEntry => ({
    id: path,
    path,
    kind: 'asset',
    attributes: { format },
    blob: { sha256: 'a'.repeat(64), bytes: 1 },
  })
  expect(
    attachmentMarkdown(entry('statement/problem.md', 'markdown'), entry('attachments/a (b).png')),
  ).toBe('![a (b).png](../attachments/a%20%28b%29.png)')
  expect(
    attachmentMarkdown(entry('statement/problem.tex', 'tex'), entry('attachments/a.png')),
  ).toContain('\\detokenize{../attachments/a.png}')
  expect(() =>
    attachmentMarkdown(entry('statement/problem.tex', 'tex'), entry('attachments/a}.png')),
  ).toThrow('不适合')
})
