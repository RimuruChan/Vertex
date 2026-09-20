import { Unzip, UnzipInflate, zipSync } from 'fflate'
import type { DomainContentTree, DomainTreeEntry } from '@/generated/api/model'
import { canonicalTree, materialNames } from '@/lib/authoring-materials'
import { MockError } from './errors'

// Demo limits are deliberately below the real server's limits. ZIP processing
// stays local and never falls back to a real backend.
export const demoArchiveBytes = 8 << 20
const expandedLimit = 32 << 20
const fileLimit = 8 << 20
const entryLimit = 1000
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true })
const fail = (message: string): never => {
  throw new MockError(400, message)
}

export type PreparedArchive = {
  format: 'vertex' | 'luogu-data'
  hash: string
  files: Record<string, number[]>
  tree?: DomainContentTree
}
export async function sha256(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new Uint8Array(bytes).buffer)
  return [...new Uint8Array(digest)].map((value) => value.toString(16).padStart(2, '0')).join('')
}
export function portablePath(name: string) {
  if (
    !name ||
    name.length > 512 ||
    /[\\:\x00-\x1f\x7f]/.test(name) ||
    name
      .split('/')
      .some(
        (part) =>
          !part ||
          part === '.' ||
          part === '..' ||
          /[. ]$/.test(part) ||
          /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(part),
      )
  )
    fail('题包路径无效：' + name)
}
function uniquePaths(names: string[]) {
  const seen = new Set<string>()
  for (const name of names) {
    portablePath(name)
    const key = name.toLowerCase()
    if (seen.has(key)) fail('题包路径重复：' + name)
    seen.add(key)
  }
  for (const name of seen) {
    const parts = name.split('/')
    for (let index = 1; index < parts.length; index++)
      if (seen.has(parts.slice(0, index).join('/'))) fail('文件与目录路径冲突：' + name)
  }
}
const crcTable = Uint32Array.from({ length: 256 }, (_, index) => {
  let value = index
  for (let bit = 0; bit < 8; bit++) value = (value >>> 1) ^ (value & 1 ? 0xedb88320 : 0)
  return value >>> 0
})
function crc32(bytes: Uint8Array) {
  let value = 0xffffffff
  for (const byte of bytes) value = (value >>> 8) ^ crcTable[(value ^ byte) & 255]
  return (value ^ 0xffffffff) >>> 0
}

// Inspect the central directory before inflating, then bound actual streamed
// output too. Declared sizes alone are not a decompression budget.
export function readDemoZip(bytes: Uint8Array): Record<string, number[]> {
  if (bytes.length > demoArchiveBytes) fail('演示题包最大 8 MiB；大题包请使用真实后端。')
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  let end = bytes.length - 22
  while (end >= Math.max(0, bytes.length - 65557) && view.getUint32(end, true) !== 0x06054b50) end--
  if (
    end < 0 ||
    end < bytes.length - 65557 ||
    end + 22 + view.getUint16(end + 20, true) !== bytes.length
  )
    fail('ZIP 目录缺失或文件不完整')
  const count = view.getUint16(end + 10, true),
    offset = view.getUint32(end + 16, true)
  if (
    !count ||
    count > entryLimit ||
    view.getUint16(end + 4, true) ||
    view.getUint16(end + 6, true) ||
    count !== view.getUint16(end + 8, true) ||
    offset + view.getUint32(end + 12, true) !== end
  )
    fail('演示题包不支持分卷、ZIP64 或超过 1000 个条目')
  const declared = new Map<string, { size: number; crc: number }>()
  const fileNames: string[] = []
  let cursor = offset,
    total = 0
  for (let index = 0; index < count; index++) {
    if (cursor + 46 > end || view.getUint32(cursor, true) !== 0x02014b50) fail('ZIP 目录无效')
    const flags = view.getUint16(cursor + 8, true),
      method = view.getUint16(cursor + 10, true)
    const size = view.getUint32(cursor + 24, true),
      nameSize = view.getUint16(cursor + 28, true)
    const next =
      cursor + 46 + nameSize + view.getUint16(cursor + 30, true) + view.getUint16(cursor + 32, true)
    if (next > end || flags & 1 || ![0, 8].includes(method) || view.getUint16(cursor + 34, true))
      fail('不支持此 ZIP 编码或加密方式')
    const rawName = bytes.subarray(cursor + 46, cursor + 46 + nameSize)
    if (!(flags & 2048) && rawName.some((byte) => byte > 127)) fail('ZIP 文件名需要 UTF-8 编码')
    const name = decoder.decode(rawName),
      directory = name.endsWith('/')
    portablePath(directory ? name.slice(0, -1) : name)
    const mode = view.getUint32(cursor + 38, true) >>> 16
    if (mode & 0xf000 && (mode & 0xf000) !== (directory ? 0x4000 : 0x8000))
      fail('演示题包不支持链接或特殊文件')
    total += size
    if (size > fileLimit || total > expandedLimit || (directory && size))
      fail('演示题包展开后过大或目录无效')
    if (declared.has(name)) fail('题包路径重复：' + name)
    declared.set(name, { size, crc: view.getUint32(cursor + 16, true) })
    if (!directory) fileNames.push(name)
    const local = view.getUint32(cursor + 42, true)
    if (
      local + 30 > offset ||
      view.getUint32(local, true) !== 0x04034b50 ||
      view.getUint16(local + 6, true) !== flags ||
      view.getUint16(local + 8, true) !== method
    )
      fail('ZIP 文件头不一致')
    const localNameSize = view.getUint16(local + 26, true)
    const dataStart = local + 30 + localNameSize + view.getUint16(local + 28, true)
    if (
      dataStart + view.getUint32(cursor + 20, true) > offset ||
      decoder.decode(bytes.subarray(local + 30, local + 30 + localNameSize)) !== name
    )
      fail('ZIP 文件内容越界')
    cursor = next
  }
  if (cursor !== end) fail('ZIP 目录长度不一致')
  uniquePaths(fileNames)
  const files: Record<string, number[]> = Object.create(null)
  const opened = new Set<string>(),
    complete = new Set<string>()
  let inflated = 0
  const unzip = new Unzip((file) => {
    const info = declared.get(file.name) ?? fail('ZIP 流包含未声明文件')
    if (opened.has(file.name)) fail('ZIP 流包含重复文件')
    opened.add(file.name)
    let size = 0
    const chunks: Uint8Array[] = []
    file.ondata = (error, chunk, final) => {
      if (error) throw error
      size += chunk.length
      inflated += chunk.length
      if (size > info.size || size > fileLimit || inflated > expandedLimit)
        fail('ZIP 实际展开内容超过限制')
      chunks.push(chunk)
      if (final) {
        const content = new Uint8Array(size)
        let position = 0
        for (const chunk of chunks) {
          content.set(chunk, position)
          position += chunk.length
        }
        if (size !== info.size || crc32(content) !== info.crc)
          fail('ZIP 文件校验失败：' + file.name)
        complete.add(file.name)
        if (!file.name.endsWith('/')) files[file.name] = [...content]
      }
    }
    file.start()
  })
  unzip.register(UnzipInflate)
  for (let position = 0; position < bytes.length; position += 4096)
    unzip.push(bytes.subarray(position, position + 4096), position + 4096 >= bytes.length)
  if (complete.size !== declared.size) fail('ZIP 文件未完整解压')
  return files
}

function nativeTree(value: unknown): DomainContentTree {
  const tree = value as DomainContentTree
  if (
    !tree ||
    Object.keys(tree).some((key) => key !== 'entries') ||
    !Array.isArray(tree.entries) ||
    tree.entries.length > entryLimit
  )
    fail('无效的原生内容树')
  const ids = new Set<string>()
  for (const entry of tree.entries) {
    if (
      !entry ||
      Object.keys(entry).some(
        (key) => !['id', 'path', 'kind', 'attributes', 'blob'].includes(key),
      ) ||
      !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(entry.id) ||
      ids.has(entry.id) ||
      !Object.prototype.hasOwnProperty.call(materialNames, entry.kind) ||
      !entry.attributes ||
      typeof entry.attributes !== 'object' ||
      Array.isArray(entry.attributes) ||
      Object.values(entry.attributes).some((value) => typeof value !== 'string') ||
      !entry.blob ||
      Object.keys(entry.blob).some((key) => !['sha256', 'bytes'].includes(key)) ||
      !/^[a-f0-9]{64}$/.test(entry.blob.sha256) ||
      !Number.isSafeInteger(entry.blob.bytes) ||
      entry.blob.bytes < 0
    )
      fail('无效的原生材料描述')
    ids.add(entry.id)
  }
  uniquePaths(tree.entries.map((entry) => entry.path))
  return canonicalTree(tree)
}
export async function prepareDemoArchive(file: Blob): Promise<PreparedArchive> {
  if (file.size > demoArchiveBytes) fail('演示题包最大 8 MiB；大题包请使用真实后端。')
  try {
    const bytes = new Uint8Array(await file.arrayBuffer()),
      files = readDemoZip(bytes)
    const hash = await sha256(bytes)
    if (files['vertex-package.json']) {
      const manifest = JSON.parse(decoder.decode(new Uint8Array(files['vertex-package.json'])))
      if (
        manifest.schemaVersion !== 1 ||
        Object.keys(manifest).some((key) => !['schemaVersion', 'tree'].includes(key))
      )
        fail('不支持的 Vertex 归档版本')
      const tree = nativeTree(manifest.tree),
        names = new Set(['vertex-package.json'])
      for (const entry of tree.entries) {
        const name = 'blobs/' + entry.blob.sha256,
          content = files[name]
        if (
          !content ||
          content.length !== entry.blob.bytes ||
          (await sha256(new Uint8Array(content))) !== entry.blob.sha256
        )
          fail('归档材料摘要不匹配：' + entry.path)
        names.add(name)
      }
      if (Object.keys(files).some((name) => !names.has(name))) fail('原生归档包含未声明文件')
      return { format: 'vertex', hash, files, tree }
    }
    if (Object.keys(files).some((name) => name.includes('/') || !/\d.*\.(in|out|ans)$/.test(name)))
      throw new MockError(
        501,
        '演示模式支持 Vertex 原生归档和成对的平铺数据 ZIP；Kattis、Polygon 和 config.yml 请使用真实后端。',
      )
    return { format: 'luogu-data', hash, files }
  } catch (error) {
    if (error instanceof MockError) throw error
    return fail('无法读取题包，请检查 ZIP 和原生清单是否完整。')
  }
}
export async function createDemoDataArchive(
  tree: DomainContentTree,
  read: (entry: DomainTreeEntry) => number[],
) {
  const files: Record<string, Uint8Array> = Object.create(null)
  const metadata = tree.entries.find((entry) => entry.id === 'problem') ?? fail('缺少基本设置')
  const order = JSON.parse(decoder.decode(new Uint8Array(read(metadata)))).testOrder as string[]
  if (!order.length || order.length * 2 > entryLimit) fail('演示数据包的测试数量无效')
  let total = 0
  for (let index = 0; index < order.length; index++) {
    const entry =
      tree.entries.find((item) => item.id === order[index] && item.kind === 'test') ??
      fail('测试顺序引用无效')
    const test = JSON.parse(decoder.decode(new Uint8Array(read(entry))))
    if (test.timeLimitMs || test.memoryLimitKb)
      throw new MockError(501, '带 config.yml 的逐点限制请使用真实后端导出')
    for (const [kind, suffix, fileKind] of [
      ['input', 'in', 'input'],
      ['answer', 'out', 'answer'],
    ]) {
      if (test[kind].kind !== 'file')
        throw new MockError(501, '生成型数据请使用真实 Worker 检查后导出')
      const file =
        tree.entries.find((item) => item.id === test[kind].entry && item.kind === fileKind) ??
        fail('测试文件引用无效')
      const bytes = new Uint8Array(read(file))
      total += bytes.length
      if (bytes.length > fileLimit || total > expandedLimit) fail('数据超过演示归档限制')
      files[`${String(index + 1).padStart(4, '0')}.${suffix}`] = bytes
    }
  }
  const bytes = zipSync(files, { level: 6, mtime: new Date(1980, 0, 1) })
  if (bytes.length > demoArchiveBytes) fail('演示题包最大 8 MiB')
  return [...bytes]
}

export async function createDemoArchive(
  tree: DomainContentTree,
  read: (entry: DomainTreeEntry) => number[],
) {
  const files: Record<string, Uint8Array> = Object.create(null)
  const entries: DomainTreeEntry[] = []
  let total = 0
  for (const entry of canonicalTree(tree).entries) {
    const bytes = new Uint8Array(read(entry))
    total += bytes.length
    if (bytes.length > fileLimit || total > expandedLimit || entries.length >= entryLimit - 1)
      fail('材料超过演示题包限制，请使用真实后端导出。')
    const digest = await sha256(bytes)
    files['blobs/' + digest] = bytes
    entries.push({ ...entry, blob: { sha256: digest, bytes: bytes.length } })
  }
  files['vertex-package.json'] = encoder.encode(
    JSON.stringify({ schemaVersion: 1, tree: { entries } }),
  )
  const sorted = Object.fromEntries(Object.entries(files).sort(([a], [b]) => a.localeCompare(b)))
  const bytes = zipSync(sorted, {
    level: 6,
    mtime: new Date(1980, 0, 1),
    os: 3,
    attrs: 0o100644 << 16,
  })
  if (bytes.length > demoArchiveBytes) fail('导出超过演示题包限制，请使用真实后端。')
  return [...bytes]
}
