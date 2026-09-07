import { useEffect, useRef, useState } from 'react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoPackageMetaResponse,
  DtoReleaseResponse,
  DtoStatementResponse,
} from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime } from '@/lib/format'

export default function ReleasePanel({
  problemId,
  meta,
  statements,
  onPublished,
}: {
  problemId: string
  meta: DtoPackageMetaResponse
  statements: DtoStatementResponse[]
  onPublished: () => void
}) {
  const { getApiAdminProblemsIdReleases, postApiAdminProblemsIdPublish } = useDomainAPI()
  const toast = useToast(),
    confirm = useConfirm()
  const [versions, setVersions] = useState<DtoReleaseResponse[]>([])
  const [loading, setLoading] = useState(true),
    [error, setError] = useState<string | null>(null)
  const [reload, setReload] = useState(0),
    [publishing, setPublishing] = useState(false)
  const [language, setLanguage] = useState(meta.statementLanguage)
  const active = useRef(true)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    getApiAdminProblemsIdReleases(problemId, { signal: controller.signal })
      .then((value) => {
        if (!controller.signal.aborted) setVersions(value.items)
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(apiError(cause, '发布记录加载失败'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [problemId, meta.publishedVersion, reload])
  async function publish() {
    if (publishing || !meta.canPublish || meta.stale) return
    if (
      !(await confirm({
        title: '发布当前已保存的工作副本？',
        description: `将工作修订 r${meta.packageRevision} 与候选数据 v${meta.testdataVersion} 组合为不可变版本。当前可见性保持「${meta.visibility}」。已有比赛和评测任务不会自动升级；未保存的编辑不包含在内。`,
        confirmLabel: '发布版本',
      }))
    )
      return
    if (!active.current) return
    setPublishing(true)
    try {
      const release = await postApiAdminProblemsIdPublish(problemId, {
        revision: meta.packageRevision,
        artifactVersion: meta.testdataVersion,
        language,
      })
      if (!active.current) return
      toast.success(`已发布版本 v${release.version}`)
      onPublished()
      setReload((value) => value + 1)
    } catch (cause) {
      toast.error(apiError(cause, '发布失败，请刷新后核对工作副本与候选数据'))
    } finally {
      setPublishing(false)
    }
  }
  const changed = meta.unpublishedChanges || versions[0]?.language !== language
  return (
    <div className="flex flex-col gap-4">
      <Card className="space-y-4 p-5">
        <div>
          <h2 className="font-medium">发布已审核版本</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            工作副本和候选数据不会直接上线。发布后，题面、限制和数据一起切换；已有比赛保留绑定版本。
          </p>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="release-language">默认题面语言</Label>
            <select
              id="release-language"
              className="h-9 rounded-md border border-input bg-background px-3"
              value={language}
              onChange={(event) => setLanguage(event.target.value)}
              disabled={!meta.canPublish || publishing}
            >
              {[...new Set([meta.statementLanguage, ...statements.map((s) => s.language)])].map(
                (value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ),
              )}
            </select>
          </div>
          <Button
            loading={publishing}
            disabled={!meta.canPublish || meta.stale || !changed}
            onClick={publish}
          >
            发布版本
          </Button>
          <span className="pb-2 text-sm text-muted-foreground">
            {meta.publishedVersion ? `当前 v${meta.publishedVersion}` : '尚未发布'}
          </span>
        </div>
        <p className="text-sm text-muted-foreground">
          {!meta.canPublish
            ? '只有题目 owner 或域资源管理者可以发布。'
            : meta.stale
              ? '请先构建或导入与当前数据修订匹配的候选。'
              : !changed
                ? '已保存内容与当前发布版本一致。'
                : `将发布 r${meta.packageRevision} / 数据 v${meta.testdataVersion}，共 ${meta.testdataCases} 个测试点。`}
        </p>
      </Card>
      <Card className="p-5">
        <h2 className="mb-3 font-medium">发布记录</h2>
        {loading ? (
          <p role="status" className="text-sm text-muted-foreground">
            正在加载…
          </p>
        ) : error ? (
          <div className="flex flex-wrap items-center gap-3">
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
            <Button variant="outline" size="sm" onClick={() => setReload((v) => v + 1)}>
              重试
            </Button>
          </div>
        ) : versions.length ? (
          <ul className="divide-y divide-border">
            {versions.map((version) => (
              <li
                key={version.version}
                className="flex flex-wrap items-center justify-between gap-2 py-3 text-sm"
              >
                <span>
                  v{version.version} · r{version.revision} · 数据 v{version.artifactVersion} ·{' '}
                  {version.language}
                </span>
                <span className="text-muted-foreground">{formatDateTime(version.createdAt)}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">还没有发布版本。</p>
        )}
      </Card>
    </div>
  )
}
