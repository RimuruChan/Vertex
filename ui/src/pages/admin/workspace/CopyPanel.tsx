import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCallback, useState } from 'react'
import type { DtoProblemResponse } from '@/generated/api/model'
import { getApiDomains, postApiDomainsDomainProblemCopies } from '@/generated/api/vertex'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Link, useNavigate } from '@/domain/navigation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime } from '@/lib/format'

export default function CopyPanel({ problem }: { problem: DtoProblemResponse }) {
  const { slug } = useDomain(),
    api = useDomainAPI(),
    navigate = useNavigate(),
    confirm = useConfirm(),
    toast = useToast(),
    active = useActiveRef()
  const [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [target, setTarget] = useState(''),
    [version, setVersion] = useState(String(problem.publishedVersion || '')),
    [attribution, setAttribution] = useState(''),
    [busy, setBusy] = useState(false)
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [domains, releases, origin] = await Promise.all([
        getApiDomains({ size: 100, keyword }, { signal }),
        api.getApiAdminProblemsIdReleases(problem.id, { signal }),
        api.getApiAdminProblemsIdOrigin(problem.id, { signal }),
      ])
      return {
        domains: domains.items.filter(
          (domain) => domain.permissions.includes('problem.create') && !domain.archived,
        ),
        total: domains.total,
        releases: releases.items,
        origin: origin.origin,
      }
    },
    [api, problem.id, problem.publishedVersion, keyword],
  )
  const remote = useRemote(load),
    selected = Number(version)
  const destination = remote.data?.domains.find((domain) => domain.slug === target)
  const release = remote.data?.releases.find((item) => item.version === selected)
  async function copy() {
    if (
      busy ||
      !problem.permissions.copy ||
      !destination ||
      !Number.isSafeInteger(selected) ||
      selected <= 0 ||
      !attribution.trim()
    )
      return
    if (
      !(await confirm({
        title: `将 v${selected} 复制到「${destination.name}」？`,
        description:
          '只复制所选发布版本，不带入当前未发布修改、协作者、提交或比赛关系。副本使用独立数据和新编号，默认草稿；你将成为新 owner。',
        confirmLabel: '创建独立副本',
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      const result = await postApiDomainsDomainProblemCopies(destination.slug, {
        sourceDomain: slug,
        sourceProblem: problem.publicId,
        sourceVersion: selected,
        attribution: attribution.trim(),
      })
      if (!active.current) return
      toast.success('副本已创建，请审核后再发布')
      navigate(`/d/${result.domainSlug}/authoring/${result.problemPublicId}`)
    } catch (error) {
      if (active.current) toast.error(apiError(error, '复制失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <Card className="space-y-4 p-5">
        <h2 className="font-medium">复制已发布版本</h2>
        <p className="text-sm text-muted-foreground">
          源题包复制权限和目标域创建权限缺一不可。独立副本不跟随源题后续修改，已有版本、数据与来源记录会保留。
        </p>
        {remote.error ? (
          <div className="flex items-center gap-3">
            <p role="alert" className="text-sm text-destructive">
              {remote.error}
            </p>
            <Button variant="outline" onClick={remote.reload}>
              重试
            </Button>
          </div>
        ) : !remote.data ? (
          <p role="status">正在加载可复制版本和目标域…</p>
        ) : (
          <>
            {problem.publishedVersion === 0 ? (
              <p className="text-sm text-muted-foreground">
                请先发布一个可评测版本；私有或草稿可见性不必改成公开。
              </p>
            ) : (
              <form
                className="space-y-4"
                onSubmit={(event) => {
                  event.preventDefault()
                  void copy()
                }}
              >
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="copy-release">源发布版本</Label>
                    <Input
                      id="copy-release"
                      type="number"
                      min={1}
                      step={1}
                      required
                      value={version}
                      onChange={(event) => setVersion(event.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">
                      {release
                        ? `${release.caseCount} 个测试点 · ${formatDateTime(release.createdAt)}`
                        : '输入已经审核的版本号，服务器会再次核对。'}
                    </p>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="copy-target">目标域</Label>
                    <Select
                      value={target}
                      onValueChange={(value) => setTarget(value === '__none__' ? '' : value)}
                      required
                    >
                      <SelectTrigger id="copy-target" className="h-9">
                        <SelectValue placeholder="选择有创建权限的域" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="__none__">选择有创建权限的域</SelectItem>
                        {remote.data.domains.map((domain) => (
                          <SelectItem key={domain.id} value={domain.slug}>
                            {domain.name} · {domain.slug}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <div className="flex gap-2">
                  <Input
                    aria-label="搜索复制目标域"
                    placeholder="按域名称或标识缩小范围"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                  />
                  <Button type="button" variant="outline" onClick={() => setKeyword(query.trim())}>
                    查找目标域
                  </Button>
                </div>
                {remote.data.total > 100 && (
                  <p className="text-xs text-muted-foreground">
                    当前只加载前 100 个可见域，请输入名称缩小范围。
                  </p>
                )}
                <div className="space-y-2">
                  <Label htmlFor="copy-attribution">来源与授权说明</Label>
                  <Textarea
                    id="copy-attribution"
                    required
                    maxLength={4096}
                    placeholder="说明题目来源及本次复制用途；请遵循源材料授权。"
                    value={attribution}
                    onChange={(event) => setAttribution(event.target.value)}
                  />
                </div>
                <Button
                  type="submit"
                  loading={busy}
                  disabled={
                    !problem.permissions.copy ||
                    !destination ||
                    !attribution.trim() ||
                    selected <= 0
                  }
                >
                  创建独立副本
                </Button>
              </form>
            )}
          </>
        )}
      </Card>
      <Card className="space-y-3 p-5">
        <h2 className="font-medium">复制来源</h2>
        {remote.data?.origin ? (
          <>
            <p className="text-sm">
              <Link
                className="text-primary"
                to={`/d/${remote.data.origin.sourceDomainSlug}/problems/${remote.data.origin.sourceProblemNumber}`}
              >
                {remote.data.origin.sourceTitle}
              </Link>{' '}
              · {remote.data.origin.sourceDomainSlug}/{remote.data.origin.sourceProblemNumber} · v
              {remote.data.origin.sourceVersion}
            </p>
            <p className="whitespace-pre-wrap text-sm text-muted-foreground">
              {remote.data.origin.attribution}
            </p>
            <p className="text-xs text-muted-foreground">
              复制于 {formatDateTime(remote.data.origin.copiedAt)}
              。记录仅供包协作者读取；源题撤权或删除不改变副本。
            </p>
          </>
        ) : !remote.error && remote.data ? (
          <p className="text-sm text-muted-foreground">这道题目没有复制来源记录。</p>
        ) : (
          <p className="text-sm text-muted-foreground">来源记录尚未加载。</p>
        )}
      </Card>
    </div>
  )
}
