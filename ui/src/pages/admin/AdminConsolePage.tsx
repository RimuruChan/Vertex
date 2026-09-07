import { useCallback, useEffect, useRef, useState } from 'react'
import { Activity, Ban, CheckCircle2, Pencil, RefreshCw, Search, Server, Users } from 'lucide-react'
import {
  getApiAdminStats as getStats,
  getApiAdminUsers as listUsers,
  patchApiAdminUsersId as updateUser,
} from '@/generated/api/vertex'
import type {
  DtoAccountResponse as Account,
  DtoStatsResponse as Stats,
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
import { Input } from '@/components/ui/input'
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
import { apiError, formatRelative } from '@/lib/format'

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
 * Site-wide health and account governance. Content is managed inside its domain.
 */
export default function AdminConsolePage() {
  const toast = useToast()
  const confirm = useConfirm()
  const { user } = useAuth()

  const [stats, setStats] = useState<Stats | null>(null)
  const [loadingStats, setLoadingStats] = useState(true)
  const [statsError, setStatsError] = useState<string | null>(null)
  const [accountsError, setAccountsError] = useState<string | null>(null)
  const [loadingAccounts, setLoadingAccounts] = useState(true)

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

  const [saving, setSaving] = useState(false)
  const [pendingAction, setPendingAction] = useState<string | null>(null)
  const statsSequence = useRef(0),
    accountsSequence = useRef(0)
  useEffect(
    () => () => {
      statsSequence.current++
      accountsSequence.current++
    },
    [],
  )

  const loadStats = useCallback(async () => {
    const sequence = ++statsSequence.current
    setLoadingStats(true)
    setStatsError(null)
    try {
      const result = await getStats()
      if (sequence === statsSequence.current) setStats(result)
    } catch (error) {
      if (sequence === statsSequence.current) setStatsError(apiError(error, '站点概览加载失败'))
    } finally {
      if (sequence === statsSequence.current) setLoadingStats(false)
    }
  }, [toast])

  const loadAccounts = useCallback(async () => {
    const sequence = ++accountsSequence.current
    setLoadingAccounts(true)
    setAccountsError(null)
    try {
      const result = await listUsers({
        page: accountPage,
        size: PAGE_SIZE,
        keyword: accountKeyword || undefined,
        role: roleFilter === ANY ? undefined : (roleFilter as RoleFilter),
      })
      if (sequence === accountsSequence.current) {
        setAccounts(result.items)
        setAccountTotal(result.total)
      }
    } catch (error) {
      if (sequence === accountsSequence.current)
        setAccountsError(apiError(error, '用户列表加载失败'))
    } finally {
      if (sequence === accountsSequence.current) setLoadingAccounts(false)
    }
  }, [accountPage, accountKeyword, roleFilter, toast])

  useEffect(() => {
    void loadStats()
  }, [loadStats])

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

  const queueHealthy = (stats?.activeWorkers ?? 0) > 0 || (stats?.queuedJobs ?? 0) === 0

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">站点管理</h1>
          <p className="text-sm text-muted-foreground">站点状态与账号治理；内容管理位于各自域。</p>
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
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-4">
          {statsError ? (
            <EmptyState
              title="站点状态加载失败"
              description={statsError}
              action={<Button onClick={() => void loadStats()}>重试</Button>}
            />
          ) : loadingStats || !stats ? (
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
                {loadingAccounts ? (
                  <TableEmpty colSpan={7}>
                    <Skeleton className="h-32" />
                  </TableEmpty>
                ) : accountsError ? (
                  <TableEmpty colSpan={7}>
                    <EmptyState
                      title="账号列表加载失败"
                      description={accountsError}
                      action={<Button onClick={() => void loadAccounts()}>重试</Button>}
                    />
                  </TableEmpty>
                ) : accounts.length === 0 ? (
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
            {!loadingAccounts && !accountsError && (
              <Pagination
                page={accountPage}
                size={PAGE_SIZE}
                total={accountTotal}
                onChange={setAccountPage}
              />
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
    </div>
  )
}
