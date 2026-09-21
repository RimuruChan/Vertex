import { useEffect, useState } from 'react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useNavigate } from '@/domain/navigation'
import type { DtoProblemResponse } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { SaveButton } from '@/components/ui/save-button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Choice } from './MaterialForm'
import { apiError } from '@/lib/format'

export default function ResourcePolicyPanel({
  problem,
  onChanged,
  onBusy,
  onDirty,
}: {
  problem: DtoProblemResponse
  onChanged: (value: DtoProblemResponse) => void
  onBusy: (busy: boolean) => void
  onDirty: (dirty: boolean) => void
}) {
  const api = useDomainAPI(),
    navigate = useNavigate(),
    confirm = useConfirm()
  const [visibility, setVisibility] = useState(
      problem.visibility === 'public' ? 'public' : 'private',
    ),
    [busy, setBusy] = useState(false),
    [deleting, setDeleting] = useState(false),
    [saved, setSaved] = useState(false),
    [error, setError] = useState('')
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    onDirty(visibility !== (problem.visibility === 'public' ? 'public' : 'private'))
    return () => onDirty(false)
  }, [visibility, problem.visibility, onDirty])
  async function save() {
    setBusy(true)
    setError('')
    try {
      const value = await api.putApiAuthoringProblemsIdVisibility(problem.id, {
        visibility,
        expectedVisibility: problem.visibility,
      })
      onChanged({ ...problem, visibility: value.visibility })
      setSaved(true)
    } catch (error) {
      setError(apiError(error, '可见性保存失败'))
    } finally {
      setBusy(false)
    }
  }
  async function refresh() {
    setError('')
    try {
      const value = await api.getApiAdminProblemsId(problem.id)
      setVisibility(value.visibility === 'public' ? 'public' : 'private')
      onChanged(value)
    } catch (error) {
      setError(apiError(error, '无法读取题目权限'))
    }
  }
  async function remove() {
    if (
      !(await confirm({
        title: '删除此题目？',
        description:
          '题目及其草稿、提交和发布记录会删除。仍被比赛、题单或提交记录引用的题目无法删除。',
        confirmLabel: '删除题目',
        destructive: true,
      }))
    )
      return
    setBusy(true)
    setDeleting(true)
    setError('')
    try {
      await api.deleteApiAdminProblemsId(problem.id)
      navigate('/workspace/problems')
    } catch (error) {
      setError(apiError(error, '删除失败'))
    } finally {
      setBusy(false)
      setDeleting(false)
    }
  }
  return (
    <section
      className="max-w-3xl rounded-xl border bg-card p-5 space-y-4"
      aria-label="题目访问设置"
    >
      <div>
        <h3 className="text-sm font-medium">题目访问</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          控制已发布题目的访问。个人工作副本不会因此公开。
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <div className="w-full max-w-xs">
          <Choice
            label="可见性"
            value={visibility}
            onChange={(value) => {
              setVisibility(value)
              setSaved(false)
            }}
            disabled={busy || !problem.permissions.manageAccess}
            options={[
              ['private', '仅协作者'],
              ['public', '公开'],
            ]}
          />
        </div>
        {problem.permissions.manageAccess && (
          <SaveButton
            loading={busy && !deleting}
            saved={saved}
            disabled={busy || (visibility === problem.visibility && !saved)}
            onClick={() => void save()}
          >
            保存可见性
          </SaveButton>
        )}
      </div>
      {error && (
        <div className="space-y-2">
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
          <Button variant="outline" size="sm" disabled={busy} onClick={() => void refresh()}>
            重新读取权限
          </Button>
        </div>
      )}
      {problem.permissions.delete && (
        <div className="border-t pt-4">
          <Button
            variant="outline"
            className="text-destructive"
            loading={deleting}
            disabled={busy}
            onClick={() => void remove()}
          >
            删除题目
          </Button>
        </div>
      )}
    </section>
  )
}
