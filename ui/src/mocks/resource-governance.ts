import type { DtoAnnouncementResponse } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { mockManager, mockCan } from './domain-policy'
import { allocateReference } from './references'
import { initialWorkspace } from './authoring'
import { MockError } from './errors'
import { compareAnnouncements, isAnnouncementPinned } from '@/lib/announcements'

export function initializeGovernedResources(state: MockState) {
  state.announcements ??= state.contests[0]
    ? [
        {
          id: crypto.randomUUID(),
          publicId: '1',
          title: '周末练习赛开放报名',
          contentMd: '选一个安静的下午，一起解几道题。比赛期间可在澄清区提问。',
          pinned: true,
          published: true,
          authorName: 'Vertex',
          createdAt: state.contests[0].createdAt,
          updatedAt: state.contests[0].createdAt,
        },
      ]
    : []
  for (const notice of state.announcements) {
    if (notice.published && !notice.publishedAt) notice.publishedAt = notice.createdAt
  }
  state.tagCatalog ??= []
  state.nextTagId = Math.max(state.nextTagId ?? 0, ...state.tagCatalog.map((tag) => tag.id))
  for (const name of new Set(
    state.problems.filter((p) => p.publishedVersion).flatMap((p) => p.tags),
  ))
    if (!state.tagCatalog.some((tag) => tag.name === name))
      state.tagCatalog.push({ id: ++state.nextTagId, name, problemCount: 0 })
}

export function governanceRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
  empty = false,
) {
  initializeGovernedResources(state)
  const parts = path.split('/').filter(Boolean),
    manage = parts[1] === 'admin',
    resource = parts[manage ? 2 : 1],
    id = parts[manage ? 3 : 2],
    action = parts[4]
  const read = method === 'GET',
    manager = mockManager(state.scope, state.user)
  if (manage && (!state.user || !manager))
    throw new MockError(state.user ? 403 : 401, '需要当前域的资源管理权限')
  if (!read && (!manage || !mockCan(state.scope, state.user, 'domain.resources.manage')))
    throw new MockError(403, '没有写入权限或域已归档')
  const page = Math.max(1, Number(params.page) || 1),
    size = Math.min(100, Math.max(1, Number(params.size ?? params.limit) || 20)),
    keyword = String(params.keyword ?? '')
      .trim()
      .toLowerCase()
  const iso = new Date(now).toISOString()
  if (read && !id && empty) return { items: [], total: 0 }
  if (resource === 'announcements') {
    const rawPin = params.pinned === undefined ? undefined : String(params.pinned)
    if (
      rawPin !== undefined &&
      !['true', 'false', '1', '0', 't', 'f', 'TRUE', 'FALSE', 'True', 'False'].includes(rawPin)
    )
      throw new MockError(400, 'pinned 必须是布尔值')
    const pinFilter =
      rawPin === undefined ? undefined : ['true', '1', 't', 'TRUE', 'True'].includes(rawPin)
    const items = state
      .announcements!.filter(
        (notice) =>
          (manage || notice.published) &&
          (pinFilter === undefined || isAnnouncementPinned(notice, now) === pinFilter) &&
          (!keyword || notice.title.toLowerCase().includes(keyword) || notice.publicId === keyword),
      )
      .sort((a, b) => compareAnnouncements(a, b, now))
    if (read && !id)
      return { items: items.slice((page - 1) * size, page * size), total: items.length }
    const item = items.find((notice) => notice.id === id)
    if (id && !item) throw new MockError(404, '公告不存在')
    if (read) return item
    if (method === 'DELETE' && item) {
      state.announcements = state.announcements!.filter((notice) => notice.id !== id)
      return { status: 'deleted' }
    }
    const title = String(body.title ?? '').trim(),
      contentMd = String(body.contentMd ?? '').trim()
    if (
      !title ||
      new TextEncoder().encode(title).length > 200 ||
      new TextEncoder().encode(contentMd).length > 100000
    )
      throw new MockError(400, '公告标题或正文长度无效')
    if (
      body.pinnedUntil &&
      (typeof body.pinnedUntil !== 'string' || !Number.isFinite(Date.parse(body.pinnedUntil)))
    )
      throw new MockError(400, '置顶截止时间无效')
    const notice: DtoAnnouncementResponse = {
      id: item?.id ?? crypto.randomUUID(),
      publicId: item?.publicId ?? allocateReference(state, 'announcements'),
      title,
      contentMd,
      pinned: body.pinned === true,
      pinnedUntil:
        body.pinned === true && body.pinnedUntil
          ? new Date(String(body.pinnedUntil)).toISOString()
          : undefined,
      published: body.published !== false,
      publishedAt: item?.publishedAt ?? (body.published !== false ? iso : undefined),
      authorName: item?.authorName ?? state.user!.username,
      createdAt: item?.createdAt ?? iso,
      updatedAt: iso,
    }
    if (item) Object.assign(item, notice)
    else state.announcements!.push(notice)
    return notice
  }
  const catalogue = () =>
    state
      .tagCatalog!.map((tag) => ({
        ...tag,
        problemCount: state.problems.filter(
          (p) =>
            p.publishedVersion &&
            p.tags.includes(tag.name) &&
            (manage || p.visibility === 'public'),
        ).length,
      }))
      .filter((tag) => manage || tag.problemCount > 0)
      .sort((a, b) => a.name.localeCompare(b.name))
  if (read && !id) {
    const tags = catalogue()
    return { items: tags, total: tags.length }
  }
  const tag = state.tagCatalog!.find((tag) => String(tag.id) === id)
  if (id && !tag) throw new MockError(404, '标签不存在')
  if (read) return catalogue().find((tag) => String(tag.id) === id)
  if (method === 'POST' && !id) {
    const name = String(body.name ?? '').trim()
    if (!name || new TextEncoder().encode(name).length > 64)
      throw new MockError(400, '标签名称长度须为 1–64 字节')
    const existing = state.tagCatalog!.find((tag) => tag.name === name)
    if (existing) return catalogue().find((tag) => tag.id === existing.id)
    const added = { id: ++state.nextTagId!, name, problemCount: 0 }
    state.tagCatalog!.push(added)
    return added
  }
  if (!tag) throw new MockError(404, '标签不存在')
  const target =
    action === 'merge'
      ? state.tagCatalog!.find((tag) => tag.id === Number(body.targetId))
      : undefined
  if (action === 'merge' && (!target || target.id === tag.id))
    throw new MockError(400, '请选择本域另一个标签')
  const remove = method === 'DELETE',
    name = target?.name ?? String(body.name ?? '').trim()
  if (!remove && (!name || new TextEncoder().encode(name).length > 64))
    throw new MockError(400, '标签名称长度须为 1–64 字节')
  const merged = target ?? state.tagCatalog!.find((item) => item.name === name)
  if (merged?.id === tag.id) return catalogue().find((item) => item.id === tag.id)
  const replace = (tags: string[]) => [
    ...new Set(
      tags
        .filter((old) => !(remove && old === tag.name))
        .map((old) => (old === tag.name ? name : old)),
    ),
  ]
  for (const problem of state.problems) {
    const draft = state.problemDrafts[problem.id] ?? problem
    if (draft.tags.includes(tag.name)) {
      state.problemDrafts[problem.id] ??= structuredClone(problem)
      const workspace = (state.workspaces[problem.id] ??= initialWorkspace(problem))
      state.problemDrafts[problem.id].tags = replace(draft.tags)
      workspace.meta.packageRevision++
      workspace.meta.unpublishedChanges = true
    }
    if (problem.tags.includes(tag.name)) problem.tags = replace(problem.tags)
  }
  if (remove || merged) state.tagCatalog = state.tagCatalog!.filter((item) => item.id !== tag.id)
  else tag.name = name
  return remove
    ? { status: 'deleted' }
    : catalogue().find((item) => item.id === (merged?.id ?? tag.id))
}
