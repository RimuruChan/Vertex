import { useCallback, useEffect, useState } from 'react'
import {
  Activity,
  Ban,
  CheckCircle2,
  Megaphone,
  Merge,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Server,
  Tags,
  Trash2,
  Users,
} from 'lucide-react'
import {
  deleteApiAdminAnnouncementsId as deleteAnnouncement,
  deleteApiAdminTagsId as deleteTag,
  getApiAdminStats as getStats,
  getApiAdminTags as listTags,
  getApiAdminUsers as listUsers,
  getApiAnnouncements as listAnnouncements,
  patchApiAdminUsersId as updateUser,
  postApiAdminAnnouncements as createAnnouncement,
  postApiAdminTagsIdMerge as mergeTag,
  putApiAdminAnnouncementsId as updateAnnouncement,
  putApiAdminTagsId as renameTag,
} from '@/generated/api/vertex'
import type {
  DtoAccountResponse as Account,
  DtoAnnouncementResponse as Announcement,
  DtoStatsResponse as Stats,
  DtoTagCatalogResponse as Tag,
  DtoAccountUpdateRequestRole as AccountRole,
  GetApiAdminUsersRole as RoleFilter,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useConfirm } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime, formatRelative } from '@/lib/format'

const PAGE_SIZE = 20
const ANY = 'any'

/** One dashboard number with a label. */
function Stat({ label, value, hint }: { label: string; value: number | string; hint?: string }) {
  return (
    <Card className="flex flex-col gap-1 p-4">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-2xl font-semibold tabular-nums">{value}</span>
      {hint ? <span className="text-xs text-muted-foreground">{hint}</span> : null}
    </Card>
  )
}

/**
 * The site administration console: health at a glance, account moderation, the
 * tag catalogue and site announcements.
 */
export default function AdminConsolePage() {
  const toast = useToast()
  const confirm = useConfirm()
  const { user } = useAuth()

  const [stats, setStats] = useState<Stats | null>(null)
  const [loadingStats, setLoadingStats] = useState(true)

  const [accounts, setAccounts] = useState<Account[]>([])
  const [accountTotal, setAccountTotal] = useState(0)
  const [accountPage, setAccountPage] = useState(1)
  const [accountSearch, setAccountSearch] = useState('')
  const [accountKeyword, setAccountKeyword] = useState('')
  const [roleFilter, setRoleFilter] = useState<string>(ANY)
  const [editingAccount, setEditingAccount] = useState<Account | null>(null)
  const [draftRole, setDraftRole] = useState<AccountRole>('user')
  const [draftRating, setDraftRating] = useState(0)
  const [blockReason, setBlockReason] = useState('')

  const [tags, setTags] = useState<Tag[]>([])
  const [renaming, setRenaming] = useState<Tag | null>(null)
  const [tagName, setTagName] = useState('')
  const [merging, setMerging] = useState<Tag | null>(null)
  const [mergeTarget, setMergeTarget] = useState('')

  const [notices, setNotices] = useState<Announcement[]>([])
  const [editingNotice, setEditingNotice] = useState<Announcement | null>(null)
  const [noticeOpen, setNoticeOpen] = useState(false)
  const [noticeTitle, setNoticeTitle] = useState('')
  const [noticeBody, setNoticeBody] = useState('')
  const [noticePinned, setNoticePinned] = useState(false)
  const [noticePublished, setNoticePublished] = useState(true)
  const [saving, setSaving] = useState(false)
  const [pendingAction, setPendingAction] = useState<string | null>(null)

  const loadStats = useCallback(async () => {
    setLoadingStats(true)
    try {
      setStats(await getStats())
    } catch (error) {
      toast.error(apiError(error, '站点概览加载失败'))
    } finally {
      setLoadingStats(false)
    }
  }, [toast])

  const loadAccounts = useCallback(async () => {
    try {
      const result = await listUsers({
        page: accountPage,
        size: PAGE_SIZE,
        keyword: accountKeyword || undefined,
        role: roleFilter === ANY ? undefined : (roleFilter as RoleFilter),
      })
      setAccounts(result.items)
      setAccountTotal(result.total)
    } catch (error) {
      toast.error(apiError(error, '用户列表加载失败'))
    }
  }, [accountPage, accountKeyword, roleFilter, toast])

  const loadTags = useCallback(async () => {
    try {
      setTags((await listTags()).items)
    } catch (error) {
      toast.error(apiError(error, '标签加载失败'))
    }
  }, [toast])

  const loadNotices = useCallback(async () => {
    try {
      setNotices((await listAnnouncements({ limit: 50 })).items)
    } catch (error) {
      toast.error(apiError(error, '公告加载失败'))
    }
  }, [toast])

  useEffect(() => {
    void loadStats()
    void loadTags()
    void loadNotices()
  }, [loadStats, loadTags, loadNotices])

  useEffect(() => {
    void loadAccounts()
  }, [loadAccounts])

  function openAccount(account: Account) {
    setEditingAccount(account)
    setDraftRole(account.role as AccountRole)
    setDraftRating(account.rating)
    setBlockReason(account.disabledReason ?? '')
  }

  async function saveAccount() {
    if (!editingAccount || saving) return
    setSaving(true)
    try {
      if (draftRole !== editingAccount.role) {
        const promoting = draftRole === 'admin'
        const accepted = await confirm({
          title: promoting
            ? `授予「${editingAccount.username}」管理员权限？`
            : `撤销「${editingAccount.username}」的管理员权限？`,
          description: promoting
            ? '该账号将可以管理用户、题目、比赛和站点配置。请仅向可信用户授予此权限。'
            : '该账号将失去站点管理权限，但其普通用户数据与提交记录会保留。',
          confirmLabel: promoting ? '授予权限' : '撤销权限',
          destructive: true,
        })
        if (!accepted) return
      }

      await updateUser(editingAccount.id, { role: draftRole, rating: draftRating })
      toast.success('已保存')
      setEditingAccount(null)
      await loadAccounts()
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  async function toggleBlock(account: Account) {
    if (pendingAction) return
    const action = `${account.disabled ? 'unblock' : 'block'}:${account.id}`
    const reason =
      editingAccount?.id === account.id && blockReason.trim() ? blockReason.trim() : '违反社区规则'
    setPendingAction(action)
    try {
      const accepted = await confirm({
        title: account.disabled
          ? `解除「${account.username}」的封禁？`
          : `封禁「${account.username}」？`,
        description: account.disabled
          ? '解除后该用户可以重新登录并继续使用站点。'
          : `该账号的全部登录会话会立即失效，并且无法再次登录。封禁理由将记录为「${reason}」。`,
        confirmLabel: account.disabled ? '解除封禁' : '确认封禁',
        destructive: !account.disabled,
      })
      if (!accepted) return

      await updateUser(account.id, {
        disabled: !account.disabled,
        reason: account.disabled ? undefined : reason,
      })
      toast.success(account.disabled ? '已解封' : '已封禁,该账号的会话已全部失效')
      setEditingAccount(null)
      await loadAccounts()
    } catch (error) {
      toast.error(apiError(error, '操作失败'))
    } finally {
      setPendingAction(null)
    }
  }

  async function handleRenameTag() {
    if (!renaming || pendingAction) return
    const nextName = tagName.trim()
    const existing = tags.find((tag) => tag.id !== renaming.id && tag.name === nextName)
    const action = `rename-tag:${renaming.id}`
    setPendingAction(action)
    try {
      if (existing) {
        const accepted = await confirm({
          title: `将「${renaming.name}」合并到「${existing.name}」？`,
          description: `关联「${renaming.name}」的 ${renaming.problemCount} 道题目会改用「${existing.name}」，源标签随后永久删除。此操作无法撤销。`,
          confirmLabel: '确认合并',
          destructive: true,
        })
        if (!accepted) return
      }

      await renameTag(renaming.id, { name: nextName })
      toast.success('标签已更新')
      setRenaming(null)
      await loadTags()
    } catch (error) {
      toast.error(apiError(error, '重命名失败'))
    } finally {
      setPendingAction(null)
    }
  }

  async function handleMergeTag() {
    if (!merging || !mergeTarget || pendingAction) return
    const target = tags.find((tag) => tag.id === Number(mergeTarget))
    if (!target) return
    const action = `merge-tag:${merging.id}`
    setPendingAction(action)
    try {
      const accepted = await confirm({
        title: `将「${merging.name}」合并到「${target.name}」？`,
        description: `关联「${merging.name}」的 ${merging.problemCount} 道题目会改用「${target.name}」，源标签随后永久删除。此操作无法撤销。`,
        confirmLabel: '确认合并',
        destructive: true,
      })
      if (!accepted) return

      await mergeTag(merging.id, { targetId: Number(mergeTarget) })
      toast.success('标签已合并')
      setMerging(null)
      setMergeTarget('')
      await loadTags()
    } catch (error) {
      toast.error(apiError(error, '合并失败'))
    } finally {
      setPendingAction(null)
    }
  }

  async function handleDeleteTag(tag: Tag) {
    if (pendingAction) return
    const action = `delete-tag:${tag.id}`
    setPendingAction(action)
    try {
      const accepted = await confirm({
        title: `永久删除标签「${tag.name}」？`,
        description: `该标签会从关联的 ${tag.problemCount} 道题目中移除，题目本身不会删除。此操作无法撤销。`,
        confirmLabel: '永久删除',
        destructive: true,
      })
      if (!accepted) return

      await deleteTag(tag.id)
      toast.success('标签已删除')
      await loadTags()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setPendingAction(null)
    }
  }

  function openNotice(notice: Announcement | null) {
    setEditingNotice(notice)
    setNoticeTitle(notice?.title ?? '')
    setNoticeBody(notice?.contentMd ?? '')
    setNoticePinned(notice?.pinned ?? false)
    setNoticePublished(notice?.published ?? true)
    setNoticeOpen(true)
  }

  async function saveNotice() {
    if (!noticeTitle.trim()) {
      toast.warning('请填写公告标题')
      return
    }
    setSaving(true)
    try {
      const payload = {
        title: noticeTitle.trim(),
        contentMd: noticeBody,
        pinned: noticePinned,
        published: noticePublished,
      }
      if (editingNotice) {
        await updateAnnouncement(editingNotice.id, payload)
      } else {
        await createAnnouncement(payload)
      }
      toast.success('公告已保存')
      setNoticeOpen(false)
      await loadNotices()
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  async function removeNotice(notice: Announcement) {
    if (pendingAction) return
    const action = `delete-notice:${notice.id}`
    setPendingAction(action)
    try {
      const accepted = await confirm({
        title: `永久删除公告「${notice.title}」？`,
        description: notice.published
          ? '该公告会立即从站点移除，正文与发布状态都无法恢复。'
          : '该公告草稿及其正文会被永久删除，且无法恢复。',
        confirmLabel: '永久删除',
        destructive: true,
      })
      if (!accepted) return

      await deleteAnnouncement(notice.id)
      toast.success('公告已删除')
      await loadNotices()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setPendingAction(null)
    }
  }

  const queueHealthy = (stats?.activeWorkers ?? 0) > 0 || (stats?.queuedJobs ?? 0) === 0

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">站点管理</h1>
          <p className="text-sm text-muted-foreground">概览、用户、标签与公告</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void loadStats()}>
          <RefreshCw />
          刷新
        </Button>
      </div>

      <Tabs defaultValue="overview" className="flex flex-col gap-4">
        <TabsList>
          <TabsTrigger value="overview">
            <Activity className="size-4" />
            概览
          </TabsTrigger>
          <TabsTrigger value="users">
            <Users className="size-4" />
            用户
          </TabsTrigger>
          <TabsTrigger value="tags">
            <Tags className="size-4" />
            标签
          </TabsTrigger>
          <TabsTrigger value="announcements">
            <Megaphone className="size-4" />
            公告
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-4">
          {loadingStats || !stats ? (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {Array.from({ length: 8 }, (_, index) => (
                <Skeleton key={index} className="h-24 w-full" />
              ))}
            </div>
          ) : (
            <>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <Stat label="用户" value={stats.users} hint={`今日新增 ${stats.usersToday}`} />
                <Stat label="题目" value={stats.problems} hint={`公开 ${stats.publicProblems}`} />
                <Stat
                  label="提交"
                  value={stats.submissions}
                  hint={`今日 ${stats.submissionsToday}`}
                />
                <Stat
                  label="比赛"
                  value={stats.contests}
                  hint={`进行中 ${stats.runningContests}`}
                />
                <Stat label="题解" value={stats.editorials} />
                <Stat label="题单" value={stats.problemSets} />
              </div>

              <Card className="flex flex-col gap-3 p-4">
                <div className="flex items-center gap-2">
                  <Server className="size-4 text-muted-foreground" />
                  <p className="text-sm font-medium">判题队列</p>
                  {queueHealthy ? (
                    <Badge variant="success">正常</Badge>
                  ) : (
                    <Badge variant="destructive">没有可用的 worker</Badge>
                  )}
                </div>
                <div className="flex flex-wrap gap-x-8 gap-y-2 text-sm">
                  <span>
                    排队 <span className="font-semibold tabular-nums">{stats.queuedJobs}</span>
                  </span>
                  <span>
                    判题中 <span className="font-semibold tabular-nums">{stats.runningJobs}</span>
                  </span>
                  <span>
                    已放弃{' '}
                    <span className="font-semibold tabular-nums text-destructive">
                      {stats.deadJobs}
                    </span>
                  </span>
                  <span>
                    活跃 worker{' '}
                    <span className="font-semibold tabular-nums">{stats.activeWorkers}</span>
                  </span>
                  {stats.oldestQueued ? (
                    <span className="text-muted-foreground">
                      最早排队 {formatRelative(stats.oldestQueued)}
                    </span>
                  ) : null}
                </div>
              </Card>

              <Card className="flex flex-col gap-2 p-4">
                <p className="text-sm font-medium">最近 24 小时判定分布</p>
                {stats.verdictBreakdown.length === 0 ? (
                  <p className="text-sm text-muted-foreground">这段时间没有提交。</p>
                ) : (
                  <div className="flex flex-col gap-1">
                    {stats.verdictBreakdown.map((item) => {
                      const max = stats.verdictBreakdown[0]?.count || 1
                      return (
                        <div key={item.verdict} className="flex items-center gap-2 text-sm">
                          <span className="w-48 truncate text-muted-foreground">
                            {item.verdict}
                          </span>
                          <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted">
                            <div
                              className="h-full rounded-full bg-primary"
                              style={{ width: `${(item.count / max) * 100}%` }}
                            />
                          </div>
                          <span className="w-12 text-right tabular-nums">{item.count}</span>
                        </div>
                      )
                    })}
                  </div>
                )}
              </Card>
            </>
          )}
        </TabsContent>

        <TabsContent value="users">
          <Card className="overflow-hidden">
            <div className="flex flex-wrap items-center gap-2 border-b border-border p-4">
              <form
                className="relative"
                onSubmit={(event) => {
                  event.preventDefault()
                  setAccountPage(1)
                  setAccountKeyword(accountSearch.trim())
                }}
              >
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={accountSearch}
                  onChange={(event) => setAccountSearch(event.target.value)}
                  placeholder="用户名或邮箱"
                  className="w-56 pl-8"
                />
              </form>
              <Select
                value={roleFilter}
                onValueChange={(value) => {
                  setAccountPage(1)
                  setRoleFilter(value)
                }}
              >
                <SelectTrigger className="w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ANY}>全部角色</SelectItem>
                  <SelectItem value="user">普通用户</SelectItem>
                  <SelectItem value="admin">管理员</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>用户名</TableHead>
                  <TableHead className="hidden lg:table-cell">邮箱</TableHead>
                  <TableHead className="w-24">角色</TableHead>
                  <TableHead className="w-20 text-right">Rating</TableHead>
                  <TableHead className="w-20 text-right">通过</TableHead>
                  <TableHead className="w-32">注册</TableHead>
                  <TableHead className="w-40 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {accounts.length === 0 ? (
                  <TableEmpty colSpan={7}>
                    <EmptyState title="没有匹配的用户" />
                  </TableEmpty>
                ) : (
                  accounts.map((account) => (
                    <TableRow key={account.id}>
                      <TableCell className="font-medium">
                        <div className="flex items-center gap-1.5">
                          {account.username}
                          {account.disabled ? <Badge variant="destructive">已封禁</Badge> : null}
                          {account.id === user?.id ? (
                            <span className="text-xs text-muted-foreground">(你)</span>
                          ) : null}
                        </div>
                        {account.disabled && account.disabledReason ? (
                          <div className="text-xs text-muted-foreground">
                            {account.disabledReason}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell className="hidden truncate text-muted-foreground lg:table-cell">
                        {account.email}
                      </TableCell>
                      <TableCell>
                        <Badge variant={account.role === 'admin' ? 'warning' : 'secondary'}>
                          {account.role}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{account.rating}</TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {account.solvedCount}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatRelative(account.createdAt)}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-1">
                          <Button variant="outline" size="sm" onClick={() => openAccount(account)}>
                            <Pencil />
                            编辑
                          </Button>
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            aria-label={account.disabled ? '解封' : '封禁'}
                            className={account.disabled ? '' : 'hover:text-destructive'}
                            disabled={account.id === user?.id || pendingAction !== null}
                            aria-busy={
                              pendingAction ===
                              `${account.disabled ? 'unblock' : 'block'}:${account.id}`
                            }
                            onClick={() => void toggleBlock(account)}
                          >
                            {account.disabled ? <CheckCircle2 /> : <Ban />}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
            <Pagination
              page={accountPage}
              size={PAGE_SIZE}
              total={accountTotal}
              onChange={setAccountPage}
            />
          </Card>
        </TabsContent>

        <TabsContent value="tags">
          <Card className="overflow-hidden">
            <p className="border-b border-border p-4 text-sm font-medium">
              标签目录({tags.length})
              <span className="ml-2 font-normal text-muted-foreground">
                重命名成一个已有的名字会自动合并
              </span>
            </p>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>标签</TableHead>
                  <TableHead className="w-24 text-right">题目数</TableHead>
                  <TableHead className="w-48 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tags.length === 0 ? (
                  <TableEmpty colSpan={3}>
                    <EmptyState title="还没有标签" description="给题目打上标签后会出现在这里。" />
                  </TableEmpty>
                ) : (
                  tags.map((tag) => (
                    <TableRow key={tag.id}>
                      <TableCell className="font-medium">{tag.name}</TableCell>
                      <TableCell className="text-right tabular-nums">{tag.problemCount}</TableCell>
                      <TableCell>
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => {
                              setRenaming(tag)
                              setTagName(tag.name)
                            }}
                          >
                            重命名
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setMerging(tag)}>
                            <Merge />
                            合并
                          </Button>
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            aria-label="删除"
                            className="hover:text-destructive"
                            disabled={pendingAction !== null}
                            aria-busy={pendingAction === `delete-tag:${tag.id}`}
                            onClick={() => void handleDeleteTag(tag)}
                          >
                            <Trash2 />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </Card>
        </TabsContent>

        <TabsContent value="announcements">
          <Card className="flex flex-col gap-3 p-4">
            <div className="flex items-center justify-between">
              <p className="text-sm font-medium">站点公告</p>
              <Button size="sm" onClick={() => openNotice(null)}>
                <Plus />
                新建公告
              </Button>
            </div>
            {notices.length === 0 ? (
              <EmptyState title="还没有公告" description="公告会显示在首页顶部。" />
            ) : (
              <div className="flex flex-col gap-2">
                {notices.map((notice) => (
                  <div
                    key={notice.id}
                    className="flex items-start justify-between gap-3 rounded-lg border border-border p-3"
                  >
                    <div className="flex flex-col gap-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{notice.title}</span>
                        {notice.pinned ? <Badge variant="warning">置顶</Badge> : null}
                        {!notice.published ? <Badge variant="secondary">草稿</Badge> : null}
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {notice.authorName || '系统'} · {formatDateTime(notice.createdAt)}
                      </span>
                    </div>
                    <div className="flex items-center gap-1">
                      <Button size="sm" variant="outline" onClick={() => openNotice(notice)}>
                        <Pencil />
                        编辑
                      </Button>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label="删除"
                        className="hover:text-destructive"
                        disabled={pendingAction !== null}
                        aria-busy={pendingAction === `delete-notice:${notice.id}`}
                        onClick={() => void removeNotice(notice)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog
        open={editingAccount !== null}
        onOpenChange={(open) => !open && setEditingAccount(null)}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>编辑「{editingAccount?.username}」</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="account-role">角色</Label>
              <Select
                value={draftRole}
                onValueChange={(value) => setDraftRole(value as AccountRole)}
              >
                <SelectTrigger id="account-role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">普通用户</SelectItem>
                  <SelectItem value="admin">管理员</SelectItem>
                </SelectContent>
              </Select>
              {editingAccount?.id === user?.id ? (
                <p className="text-xs text-muted-foreground">不能取消自己的管理员权限。</p>
              ) : null}
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="account-rating">Rating</Label>
              <Input
                id="account-rating"
                type="number"
                min={0}
                max={10000}
                value={draftRating}
                onChange={(event) => setDraftRating(Number(event.target.value) || 0)}
              />
            </div>
            {editingAccount && !editingAccount.disabled ? (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="account-reason">封禁理由(封禁时记录)</Label>
                <Input
                  id="account-reason"
                  value={blockReason}
                  onChange={(event) => setBlockReason(event.target.value)}
                  placeholder="例如:重复提交他人代码"
                />
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingAccount(null)}>
              取消
            </Button>
            <Button loading={saving} onClick={saveAccount}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={renaming !== null} onOpenChange={(open) => !open && setRenaming(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>重命名标签</DialogTitle>
          </DialogHeader>
          <Input value={tagName} onChange={(event) => setTagName(event.target.value)} autoFocus />
          <p className="text-xs text-muted-foreground">如果新名字已存在,两个标签会被合并成一个。</p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenaming(null)}>
              取消
            </Button>
            <Button
              loading={pendingAction === `rename-tag:${renaming?.id}`}
              disabled={pendingAction !== null}
              onClick={handleRenameTag}
            >
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={merging !== null} onOpenChange={(open) => !open && setMerging(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>把「{merging?.name}」合并到</DialogTitle>
          </DialogHeader>
          <Select value={mergeTarget} onValueChange={setMergeTarget}>
            <SelectTrigger>
              <SelectValue placeholder="选择目标标签" />
            </SelectTrigger>
            <SelectContent>
              {tags
                .filter((tag) => tag.id !== merging?.id)
                .map((tag) => (
                  <SelectItem key={tag.id} value={String(tag.id)}>
                    {tag.name}({tag.problemCount})
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            源标签的题目会全部转到目标标签,源标签随后被删除。
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setMerging(null)}>
              取消
            </Button>
            <Button
              loading={pendingAction === `merge-tag:${merging?.id}`}
              onClick={handleMergeTag}
              disabled={!mergeTarget || pendingAction !== null}
            >
              合并
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={noticeOpen} onOpenChange={setNoticeOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingNotice ? '编辑公告' : '新建公告'}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="notice-title">标题</Label>
              <Input
                id="notice-title"
                value={noticeTitle}
                onChange={(event) => setNoticeTitle(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="notice-body">正文(Markdown)</Label>
              <Textarea
                id="notice-body"
                rows={10}
                value={noticeBody}
                onChange={(event) => setNoticeBody(event.target.value)}
              />
            </div>
            <div className="flex flex-wrap gap-4">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="size-4 accent-primary"
                  checked={noticePinned}
                  onChange={(event) => setNoticePinned(event.target.checked)}
                />
                置顶
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="size-4 accent-primary"
                  checked={noticePublished}
                  onChange={(event) => setNoticePublished(event.target.checked)}
                />
                立即发布
              </label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setNoticeOpen(false)}>
              取消
            </Button>
            <Button loading={saving} onClick={saveNotice}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
