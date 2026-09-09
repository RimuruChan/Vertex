import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useEffect, useRef, useState } from 'react'
import type { DtoContestProblemResponse } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { ContestPanel as Card } from '@/components/contest/ContestPageLayout'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function ProblemVersions({
  contestId,
  problems,
  onChanged,
}: {
  contestId: string
  problems: DtoContestProblemResponse[]
  onChanged: () => void
}) {
  const { putApiContestsIdProblemsProblemIdVersion } = useDomainAPI()
  const [selected, setSelected] = useState(problems[0]?.problemId ?? '')
  const [version, setVersion] = useState(''),
    [busy, setBusy] = useState(false)
  const confirm = useConfirm(),
    toast = useToast()
  const problem = problems.find((p) => p.problemId === selected)
  const target = Number(version)
  const active = useRef(true)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  async function adopt() {
    if (
      !problem ||
      busy ||
      !Number.isSafeInteger(target) ||
      target <= 0 ||
      target === problem.version
    )
      return
    if (
      !(await confirm({
        title: `将 ${problem.label} 从 v${problem.version} 切换到 v${target}？`,
        description:
          '需要你能访问该题目发布版本。新的提交和后续发起的重测使用新版本；已排队、正在运行和已经完成的评测不会自动改变。核对后可另行发起重测。',
        confirmLabel: '采用此版本',
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await putApiContestsIdProblemsProblemIdVersion(contestId, problem.problemId, {
        version: target,
        expectedVersion: problem.version,
      })
      if (!active.current) return
      toast.success('比赛题目版本已更新；已有结果未重测')
      setVersion('')
      onChanged()
    } catch (cause) {
      toast.error(apiError(cause, '版本切换失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card className="space-y-3 p-4">
      <h3 className="text-sm font-medium">比赛题目版本</h3>
      <p className="text-xs text-muted-foreground">
        题目发布新版不会自动升级比赛。需要更新数据时，先确认题目 owner 提供的版本，再单独发起重测。
      </p>
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1.5">
          <Label htmlFor="contest-version-problem">比赛题目</Label>
          <Select value={selected} onValueChange={(value) => setSelected(value)}>
            <SelectTrigger id="contest-version-problem" className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {problems.map((p) => (
                <SelectItem key={p.problemId} value={p.problemId}>
                  {p.label} · {p.title} · v{p.version}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex w-32 flex-col gap-1.5">
          <Label htmlFor="contest-version-target">目标版本号</Label>
          <Input
            id="contest-version-target"
            type="number"
            min={1}
            step={1}
            value={version}
            onChange={(event) => setVersion(event.target.value)}
          />
        </div>
        <Button
          variant="outline"
          loading={busy}
          disabled={
            !problem || !Number.isSafeInteger(target) || target <= 0 || target === problem.version
          }
          onClick={adopt}
        >
          采用版本
        </Button>
      </div>
    </Card>
  )
}
