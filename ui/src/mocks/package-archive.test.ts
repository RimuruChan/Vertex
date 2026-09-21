import { describe, expect, it } from 'vitest'
import { writeFileSync } from 'node:fs'
import { zipSync, strToU8 } from 'fflate'
import { createDemoArchive, prepareDemoArchive, readDemoZip, sha256 } from './package-archive'
import { preparePackageRequest } from './package-request'
import { authoringFixture } from './authoring-fixture'
import { createAuthoringExample } from './authoring-example'
import { createFixtures } from './fixtures'
import { createMockAPI } from './api'
import { adminUser, demoUser } from './identities'
import type {
  DomainImportReceipt,
  DomainPackageExport,
  DomainWorkingCopy,
} from '@/generated/api/model'

const zip = (files: Record<string, string>) =>
  new Blob([
    zipSync(
      Object.fromEntries(Object.entries(files).map(([name, value]) => [name, strToU8(value)])),
    ),
  ])

describe('bounded demo package archives', () => {
  it('exports deterministic native ZIPs with real digests and preserves binary bytes', async () => {
    const data = [0, 255, 13, 10, 42]
    const tree = {
      entries: [
        {
          id: 'input',
          kind: 'input',
          path: 'data/1.in',
          attributes: {},
          blob: { sha256: 'mock', bytes: data.length },
        },
      ],
    }
    const first = await createDemoArchive(tree, () => data)
    expect(await createDemoArchive(tree, () => data)).toEqual(first)
    const imported = await prepareDemoArchive(new Blob([new Uint8Array(first)]))
    expect(imported.format).toBe('vertex')
    expect(imported.tree?.entries[0].blob).toEqual({
      sha256: await sha256(new Uint8Array(data)),
      bytes: data.length,
    })
    expect(imported.files['blobs/' + imported.tree!.entries[0].blob.sha256]).toEqual(data)
  })
  it('exports flat data through the same authorized request flow', async () => {
    const { api, path } = authoringFixture()
    const request = await preparePackageRequest(api, {
      method: 'POST',
      path: path + '/exports',
      body: { format: 'luogu-data' },
    })
    const exported = api.handle(request) as DomainPackageExport
    const blob = api.handle({ method: 'GET', path: path + '/blobs/' + exported.file.sha256 }) as {
      mockBlob: number[]
    }
    const files = readDemoZip(new Uint8Array(blob.mockBlob))
    expect(Object.keys(files).sort()).toEqual(['0001.in', '0001.out'])
    expect(new TextDecoder().decode(new Uint8Array(files['0001.in']))).toBe('1 2\n')
    expect(exported.issues[0].code).toBe('data.only')
    expect((await prepareDemoArchive(new Blob([new Uint8Array(blob.mockBlob)]))).format).toBe(
      'luogu-data',
    )
  })
  it('rejects traversal, case collisions, missing blobs, links, damaged CRC and truncated archives', async () => {
    await expect(prepareDemoArchive(zip({ '../1.in': '1' }))).rejects.toThrow('路径')
    await expect(prepareDemoArchive(zip({ '1.in': '1', '1.IN': '1' }))).rejects.toThrow('重复')
    await expect(
      prepareDemoArchive(
        zip({
          'vertex-package.json': JSON.stringify({
            schemaVersion: 1,
            tree: {
              entries: [
                {
                  id: 'file',
                  kind: 'input',
                  path: 'data/1.in',
                  attributes: {},
                  blob: { sha256: 'a'.repeat(64), bytes: 1 },
                },
              ],
            },
          }),
        }),
      ),
    ).rejects.toThrow('摘要')
    const linked = zipSync({ '1.in': [strToU8('target'), { os: 3, attrs: 0o120777 << 16 }] })
    expect(() => readDemoZip(linked)).toThrow('链接')
    const damaged = new Uint8Array(await zip({ '1.in': 'hello' }).arrayBuffer())
    const view = new DataView(damaged.buffer),
      directory = view.getUint32(damaged.length - 6, true)
    view.setUint32(directory + 16, 0, true)
    expect(() => readDemoZip(damaged)).toThrow('校验')
    expect(() => readDemoZip(damaged.slice(0, -1))).toThrow()
  })
  it('rejects compressed expansion above the demo budget', () => {
    const archive = zipSync({ '1.in': new Uint8Array((8 << 20) + 1) })
    expect(() => readDemoZip(archive)).toThrow('过大')
  })
  it('bounds actual expansion even when the declared uncompressed size is forged', () => {
    const archive = zipSync({ '1.in': strToU8('repeated input '.repeat(400)) })
    const view = new DataView(archive.buffer)
    const directory = view.getUint32(archive.length - 6, true)
    view.setUint32(directory + 24, 1, true)
    view.setUint32(22, 1, true)
    expect(() => readDemoZip(archive)).toThrow('实际展开')
  })
})

describe('demo package workflow', () => {
  it('exports the complete authoring example as a portable native package', async () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const number = createAuthoringExample(api, 'official')
    const path = `/api/domains/official/authoring/problems/${number}`
    const exported = api.handle(
      await preparePackageRequest(api, {
        method: 'POST',
        path: path + '/exports',
        body: { format: 'vertex', revision: 1 },
      }),
    ) as DomainPackageExport
    const data = api.handle({ method: 'GET', path: path + '/blobs/' + exported.file.sha256 }) as {
      mockBlob: number[]
    }
    const archive = new Uint8Array(data.mockBlob)
    const plan = await prepareDemoArchive(new Blob([archive]))
    expect(plan.tree?.entries.filter((entry) => entry.kind === 'test')).toHaveLength(6)
    // Optional artifact used by the Go importer interoperability test and UI QA.
    if (process.env.VERTEX_MOCK_ARCHIVE) writeFileSync(process.env.VERTEX_MOCK_ARCHIVE, archive)
  })
  it('previews privately, merges data atomically and replays application without a commit', async () => {
    const f = authoringFixture(),
      { api, path } = f
    const before = f.copy(),
      version = f.problem.publishedVersion
    const request = await preparePackageRequest(api, {
      method: 'POST',
      path: path + '/imports',
      body: {
        etag: before.etag,
        file: zip({ '10.in': '10', '10.ans': '20', '2.in': '2', '2.out': '4' }),
      },
    })
    const receipt = api.handle(request) as DomainImportReceipt
    expect(receipt.plan.scope).toBe('data')
    expect(f.copy()).toEqual(before)
    const apply = () =>
      api.handle({
        method: 'POST',
        path: path + '/imports/' + receipt.id + '/apply',
        body: { etag: receipt.etag },
      }) as DomainWorkingCopy
    const changed = apply()
    expect(changed.etag).not.toBe(before.etag)
    expect(changed.headRevision).toBeUndefined()
    expect(f.problem.publishedVersion).toBe(version)
    expect(changed.tree.entries.find((entry) => entry.id === 'source')).toEqual(
      before.tree.entries.find((entry) => entry.id === 'source'),
    )
    expect(apply()).toEqual(changed)
    f.save('source', '// later private code')
    expect(apply()).toEqual(f.copy())
    expect(api.handle({ method: 'GET', path: path + '/commits' })).toEqual({ items: [] })
    const preparedAgain = await preparePackageRequest(api, {
      ...request,
      body: {
        etag: f.copy().etag,
        file: zip({ '10.in': '10', '10.ans': '20', '2.in': '2', '2.out': '4' }),
      },
    })
    const again = api.handle(preparedAgain) as DomainImportReceipt
    expect(again.plan.tree).toEqual(f.copy().tree)
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'editor' },
    })
    api.state.user = { ...demoUser }
    expect(() => api.handle({ method: 'GET', path: path + '/imports/' + receipt.id })).toThrow(
      '不存在',
    )
  })
  it('rejects stale or expired preflight without replacing a newer copy', async () => {
    let now = Date.UTC(2026, 8, 19)
    const api = createMockAPI(createFixtures(now), () => now),
      f = authoringFixture(api)
    const preview = async () =>
      api.handle(
        await preparePackageRequest(api, {
          method: 'POST',
          path: f.path + '/imports',
          body: { etag: f.copy().etag, file: zip({ '1.in': '1', '1.out': '2' }) },
        }),
      ) as DomainImportReceipt
    const first = await preview()
    f.save('source', 'newer source')
    const newer = f.copy()
    expect(() =>
      api.handle({
        method: 'POST',
        path: f.path + '/imports/' + first.id + '/apply',
        body: { etag: first.etag },
      }),
    ).toThrow('已变化')
    expect(f.copy()).toEqual(newer)
    const second = await preview()
    now += 3600001
    expect(() =>
      api.handle({
        method: 'POST',
        path: f.path + '/imports/' + second.id + '/apply',
        body: { etag: second.etag },
      }),
    ).toThrow('过期')
    expect(f.copy()).toEqual(newer)
  })
  it('exports a shared revision for readers and rechecks authorization after preparation', async () => {
    const f = authoringFixture(),
      { api, path } = f
    const revision = f.commit().revision
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'reader' },
    })
    api.state.user = { ...demoUser }
    const request = {
      method: 'POST',
      path: path + '/exports',
      body: { format: 'vertex', revision },
    }
    const staged = await preparePackageRequest(api, request)
    const result = api.handle(staged) as DomainPackageExport
    const blob = api.handle({ method: 'GET', path: path + '/blobs/' + result.file.sha256 }) as {
      mockBlob: number[]
    }
    expect((await prepareDemoArchive(new Blob([new Uint8Array(blob.mockBlob)]))).format).toBe(
      'vertex',
    )
    api.state.user = { ...adminUser }
    const grants = api.handle({
      method: 'GET',
      path: '/api/domains/official/admin/problems/1000/access',
    }) as { items: { id: number; userId: string }[] }
    api.handle({
      method: 'DELETE',
      path:
        '/api/domains/official/admin/problems/1000/access/' +
        grants.items.find((item) => item.userId === demoUser.id)!.id,
    })
    api.state.user = { ...demoUser }
    expect(() => api.handle(staged)).toThrow('权限')
  })
})
