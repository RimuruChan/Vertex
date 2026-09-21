import { expect, it, vi } from 'vitest'
import type { DomainTreeEntry } from '@/generated/api/model'
import { programSourceReader } from './program-source-reader'
const source = (id: string, bytes = 10) => ({ id, blob: { sha256: id, bytes } }) as DomainTreeEntry
it('loads only requested code and reuses immutable content across switches', async () => {
  const fetch = vi.fn(async (id: string) => new Blob([id]))
  const read = programSourceReader(fetch)
  expect(await read(source('main'))).toBe('main')
  expect(fetch.mock.calls).toEqual([['main']])
  await Promise.all([read(source('helper')), read(source('helper'))])
  await read(source('main'))
  expect(fetch.mock.calls).toEqual([['main'], ['helper']])
})
it('retries failed requests and does not preload oversized or unsaved companions', async () => {
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue(new Blob(['ok']))
  const read = programSourceReader(fetch)
  await expect(read(source('main'))).rejects.toThrow('offline')
  expect(await read(source('main'))).toBe('ok')
  expect(await read(source('huge', 2 * 1024 * 1024))).toBeUndefined()
  expect(await read(source(''))).toBeUndefined()
  expect(fetch).toHaveBeenCalledTimes(2)
})
