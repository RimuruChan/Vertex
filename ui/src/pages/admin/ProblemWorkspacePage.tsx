import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, Database, Eye, RefreshCw } from 'lucide-react'
import {
  getApiAdminProblemsIdPackage as getWorkspace,
  postApiAdminProblemsIdBuilds as startBuild,
  postApiAdminProblemsIdBuildsBuildIdCancel as cancelBuild,
} from '@/generated/api/vertex'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { PageSpinner } from '@/components/ui/misc'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime, formatMemory, formatTime } from '@/lib/format'
import BuildPanel from './workspace/BuildPanel'
import FilesPanel from './workspace/FilesPanel'
import StatementPanel from './workspace/StatementPanel'
import TestsPanel from './workspace/TestsPanel'
import { isBuildActive, type Workspace } from './workspace/types'

/** How often a running build refreshes. Progress is advisory, not a stream. */
const BUILD_POLL_MS = 1500

/**
 * The Polygon-style authoring workspace: statement, package sources, test plan
 * and build console for one problem. Every tab edits the package; only a
 * successful build publishes testdata to the judge.
 */
export default function ProblemWorkspacePage() {
  const { id = '' } = useParams()
  const toast = useToast()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [loading, setLoading] = useState(true)
  const [starting, setStarting] = useState(false)
  const pollTimer = useRef<number | null>(null)

  const load = useCallback(
    async (silent = false) => {
      if (!silent) setLoading(true)
      try {
        setWorkspace(await getWorkspace(id))
      } catch (error) {
        toast.error(apiError(error, '题目包加载失败'))
      } finally {
        if (!silent) setLoading(false)
      }
    },
    [id, toast],
  )

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  // A running build is polled until it reaches a terminal state, then once more
  // so the published testdata summary reflects what the build produced.
  useEffect(() => {
    if (!isBuildActive(workspace?.latestBuild)) {
      if (pollTimer.current) {
        window.clearTimeout(pollTimer.current)
        pollTimer.current = null
      }
      return
    }
    pollTimer.current = window.setTimeout(() => void load(true), BUILD_POLL_MS)
    return () => {
      if (pollTimer.current) window.clearTimeout(pollTimer.current)
    }
  }, [workspace, load])

  async function handleStartBuild() {
    setStarting(true)
    try {
      await startBuild(id)
      toast.success('构建已排队')
      await load(true)
    } catch (error) {
      toast.error(apiError(error, '无法开始构建'))
      await load(true)
    } finally {
      setStarting(false)
    }
  }

  async function handleCancelBuild() {
    const build = workspace?.latestBuild
    if (!build) return
    try {
      await cancelBuild(id, build.id)
      toast.success('已取消构建')
      await load(true)
    } catch (error) {
      toast.error(apiError(error, '取消失败'))
    }
  }

  if (loading || !workspace) return <PageSpinner />

  const meta = workspace.meta

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <Button variant="ghost" size="sm" className="w-fit px-0 hover:bg-transparent" asChild>
            <Link to="/admin/problems">
              <ArrowLeft />
              返回题目管理
            </Link>
          </Button>
          <h1 className="text-xl font-semibold tracking-tight">{meta.title || '未命名题目'}</h1>
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <Badge variant={meta.visibility === 'public' ? 'success' : 'secondary'}>
              {meta.visibility}
            </Badge>
            <span>{formatTime(meta.timeLimitMs)}</span>
            <span>{formatMemory(meta.memoryLimitKb)}</span>
            <span>包版本 {meta.packageRevision}</span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => void load()}>
            <RefreshCw />
            刷新
          </Button>
          {meta.visibility === 'public' ? (
            <Button variant="outline" size="sm" asChild>
              <Link to={`/problems/${id}`}>
                <Eye />
                查看公开页
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      <Card className="flex flex-wrap items-center gap-x-6 gap-y-2 p-4 text-sm">
        <div className="flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" />
          <span className="font-medium">已发布测试数据</span>
        </div>
        <span className="text-muted-foreground">
          {meta.testdataCases > 0
            ? `${meta.testdataCases} 个测试点 · ${meta.testdataChecker} · v${meta.testdataVersion}`
            : '尚未发布'}
        </span>
        {meta.lastBuiltAt ? (
          <span className="text-muted-foreground">最近构建 {formatDateTime(meta.lastBuiltAt)}</span>
        ) : null}
        {meta.stale ? (
          <Badge variant="warning">题目包已修改,需要重新构建</Badge>
        ) : meta.testdataCases > 0 ? (
          <Badge variant="success">与题目包一致</Badge>
        ) : null}
      </Card>

      <Tabs defaultValue="statement" className="flex flex-col gap-4">
        <TabsList>
          <TabsTrigger value="statement">题面</TabsTrigger>
          <TabsTrigger value="files">文件</TabsTrigger>
          <TabsTrigger value="tests">测试点 ({workspace.tests.length})</TabsTrigger>
          <TabsTrigger value="build">构建</TabsTrigger>
        </TabsList>

        <TabsContent value="statement">
          <StatementPanel
            problemId={id}
            statements={workspace.statements}
            primaryLanguage={meta.statementLanguage}
            onSaved={() => void load(true)}
          />
        </TabsContent>

        <TabsContent value="files">
          <FilesPanel problemId={id} files={workspace.files} onChanged={() => void load(true)} />
        </TabsContent>

        <TabsContent value="tests">
          <TestsPanel
            problemId={id}
            tests={workspace.tests}
            files={workspace.files}
            onChanged={() => void load(true)}
          />
        </TabsContent>

        <TabsContent value="build">
          <BuildPanel
            build={workspace.latestBuild}
            issues={workspace.issues}
            starting={starting}
            onStart={handleStartBuild}
            onCancel={handleCancelBuild}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
