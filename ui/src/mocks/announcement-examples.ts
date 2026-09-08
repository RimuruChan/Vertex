import type { DtoAnnouncementResponse } from '@/generated/api/model'
import type { createMockAPI } from './api'

// Explicit demo action: leave existing notices intact and add each example once.
export function addAnnouncementExamples(api: ReturnType<typeof createMockAPI>, slug: string) {
  const now = Date.now(),
    day = 86400000
  const samples = [
    {
      title: '本周赛事提醒：报名将在周五晚截止',
      age: 2,
      pinned: true,
      until: now + 7 * day,
      body: '本周练习赛报名已开放。请提前完成报名，比赛期间可以在答疑区提问。',
    },
    {
      title: '新手指南：从第一份 Accepted 开始',
      age: 0,
      pinned: false,
      body: '从一道基础题开始，熟悉代码编辑、提交和查看评测结果的流程。',
    },
    {
      title: '题单更新 · 动态规划基础路线',
      age: 1,
      pinned: false,
      body: '新的动态规划练习路线已上线，按知识点逐步完成练习。',
    },
    {
      title: '评测环境维护已完成',
      age: 3,
      pinned: false,
      body: '本次维护已完成，可以正常提交题目。感谢耐心等待。',
    },
    {
      title: '上周训练营活动回顾',
      age: 5,
      pinned: true,
      until: now - day,
      body: '这是一条已结束置顶的公告示例。截止时间到达后，公告仍保留在普通列表中。',
    },
  ]
  for (const sample of samples) {
    const found = api.handle({
      method: 'GET',
      path: `/api/domains/${slug}/admin/announcements`,
      params: { keyword: sample.title },
    }) as { total: number }
    if (found.total) continue
    const created = api.handle({
      method: 'POST',
      path: `/api/domains/${slug}/admin/announcements`,
      body: {
        title: sample.title,
        contentMd: sample.body,
        published: true,
        pinned: sample.pinned,
        pinnedUntil: sample.until ? new Date(sample.until).toISOString() : undefined,
      },
    }) as DtoAnnouncementResponse
    const data = slug === 'official' ? api.state : api.state.domainSpaces![slug]
    const stored = data.announcements!.find((item) => item.id === created.id)!
    stored.createdAt =
      stored.publishedAt =
      stored.updatedAt =
        new Date(now - sample.age * day).toISOString()
  }
}
