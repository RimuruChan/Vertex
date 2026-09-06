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
          title: '替换已发布测试数据？',
          description: `「${file.name}」将替换当前测试数据版本，后续提交使用新数据。`,
          confirmLabel: '确认替换',
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
          也可以上传已有的 ZIP 测试数据包。导入操作会更新已发布版本。
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
