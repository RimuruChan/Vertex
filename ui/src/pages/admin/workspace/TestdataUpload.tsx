import { useRef, useState } from 'react'
import { postApiAdminProblemsIdTestdata as upload } from '@/generated/api/vertex'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function TestdataUpload({
  problemId,
  onChanged,
}: {
  problemId: string
  onChanged: () => void
}) {
  const input = useRef<HTMLInputElement>(null)
  const confirm = useConfirm()
  const toast = useToast()
  const [busy, setBusy] = useState(false)
  async function importFile(file: File) {
    if (busy) return
    setBusy(true)
    try {
      if (
        !(await confirm({
          title: '导入新的候选数据？',
          description: `「${file.name}」将替换工作副本的候选数据。只有在发布页确认后才用于新评测，既有版本不受影响。`,
          confirmLabel: '导入候选',
          destructive: true,
        }))
      )
        return
      const result = await upload(problemId, { file, checker: 'diff' })
      toast.success(`已导入 ${result.caseCount} 个测试点`)
      onChanged()
    } catch (error) {
      toast.error(apiError(error, '导入失败'))
    } finally {
      setBusy(false)
      if (input.current) input.current.value = ''
    }
  }
  return (
    <section className="surface-panel mb-4 flex flex-wrap items-center justify-between gap-4 p-4">
      <div>
        <h2 className="text-sm font-medium">导入测试数据</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          可以上传已有 ZIP 测试数据包。导入只准备候选，不更新已发布版本。
          {import.meta.env.VITE_MOCK === 'true'
            ? '演示模式仅模拟导入状态，不解析、校验或执行 ZIP 内容。'
            : null}
        </p>
      </div>
      <input
        ref={input}
        type="file"
        accept=".zip"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0]
          if (file) void importFile(file)
        }}
      />
      <Button variant="outline" loading={busy} onClick={() => input.current?.click()}>
        选择 ZIP 文件
      </Button>
    </section>
  )
}
