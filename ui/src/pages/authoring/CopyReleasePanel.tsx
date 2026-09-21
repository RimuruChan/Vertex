import { useEffect, useState } from 'react'
import type {
  DomainCommitRelease,
  DtoCopyOriginResponse,
  DtoDomainResponse,
} from '@/generated/api/model'
import { getApiDomains } from '@/generated/api/vertex'
import { bindDomainAPI } from '@/domain/api'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/input'
import { Choice, Field } from './MaterialForm'
import { apiError } from '@/lib/format'
import { Copy } from 'lucide-react'

export default function CopyReleasePanel({
  problemId,
  releases,
  disabled,
  onBusy,
}: {
  problemId: string
  releases: DomainCommitRelease[]
  disabled: boolean
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI(),
    { slug } = useDomain(),
    navigate = useNavigate()
  const [domains, setDomains] = useState<DtoDomainResponse[]>([]),
    [target, setTarget] = useState(''),
    [version, setVersion] = useState(''),
    [note, setNote] = useState('')
  const [origin, setOrigin] = useState<DtoCopyOriginResponse>(),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    let live = true
    api
      .getApiAuthoringProblemsIdOrigin(problemId)
      .then((value) => {
        if (live) setOrigin(value.origin)
      })
      .catch((error) => {
        if (live) setError(apiError(error, '读取来源记录失败'))
      })
    return () => {
      live = false
    }
  }, [api, problemId])
  useEffect(() => {
    if (!releases.length) return
    let live = true
    getApiDomains({ size: 100 })
      .then((value) => {
        if (!live) return
        const available = value.items.filter(
          (item) => item.canEnter && !item.archived && item.permissions.includes('problem.create'),
        )
        setDomains(available)
        setTarget((current) => current || available[0]?.slug || '')
      })
      .catch((error) => {
        if (live) setError(apiError(error, '目标域加载失败'))
      })
    setVersion((current) => current || String(releases[0].version))
    return () => {
      live = false
    }
  }, [releases.length])
  async function copy() {
    if (busy || disabled) return
    if (!target || !version) {
      setError('选择目标域和发布版本。')
      return
    }
    if (!note.trim()) {
      setError('填写复制说明和来源。')
      return
    }
    setBusy(true)
    setError('')
    try {
      const result = await bindDomainAPI(target).postApiAuthoringProblemCopies({
        sourceDomain: slug,
        sourceProblem: problemId,
        sourceVersion: Number(version),
        attribution: note.trim(),
      })
      navigate(`/d/${result.domainSlug}/authoring/${result.problemId}/statement`)
    } catch (error) {
      setError(apiError(error, '复制失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      {origin && (
        <section className="rounded-xl border p-5 space-y-2" aria-label="题目来源">
          <h3 className="text-sm font-medium">来源记录</h3>
          <p className="text-sm">
            {origin.sourceTitle} · {origin.sourceDomainSlug} / #{origin.sourceProblemNumber} · v
            {origin.sourceVersion}
          </p>
          <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">
            {origin.attribution}
          </p>
        </section>
      )}
      {releases.length > 0 && (
        <details className="rounded-xl border bg-card p-5">
          <summary className="cursor-pointer text-sm font-medium">复制为独立题目</summary>
          <div className="mt-4 max-w-2xl space-y-4">
            <p className="text-sm leading-6 text-muted-foreground">
              复制指定发布版本的材料和已验证数据，在目标域创建私人草稿。源题目之后的修改不会影响副本。
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <Choice
                label="来源发布版本"
                value={version}
                onChange={setVersion}
                disabled={busy || disabled}
                options={releases.map((item) => [
                  String(item.version),
                  `v${item.version} · r${item.revision}`,
                ])}
              />
              <Choice
                label="目标域"
                value={target}
                onChange={setTarget}
                disabled={busy || disabled}
                options={domains.map((item) => [item.slug, `${item.name} / ${item.slug}`])}
              />
            </div>
            <Field label="复制说明与来源">
              <Textarea
                aria-label="复制说明与来源"
                value={note}
                maxLength={4096}
                onChange={(event) => setNote(event.target.value)}
                disabled={busy || disabled}
                placeholder="例如：从训练域复制，用于下一场练习赛。"
              />
            </Field>
            {!domains.length && (
              <p className="text-xs text-muted-foreground">需要在目标域拥有创建题目的权限。</p>
            )}
            <Button
              variant="outline"
              loading={busy}
              disabled={busy || disabled || !target}
              onClick={() => void copy()}
            >
              <Copy />
              复制到目标域
            </Button>
          </div>
        </details>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}
