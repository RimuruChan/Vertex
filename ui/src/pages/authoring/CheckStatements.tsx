import { lazy, Suspense, useCallback, useState } from 'react'
import { Download, Eye, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type { DomainStatementPreview } from '@/generated/api/model'
import { apiError } from '@/lib/format'

const PdfStatement = lazy(() => import('@/components/PdfStatement'))

export default function CheckStatements({
  problemId,
  checkId,
  statements,
}: {
  problemId: string
  checkId: string
  statements: DomainStatementPreview[]
}) {
  const api = useDomainAPI()
  const [selected, setSelected] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const load = useCallback(
    () =>
      api.getApiAuthoringProblemsIdChecksCheckIdStatementsStatementId(problemId, checkId, selected),
    [api, problemId, checkId, selected],
  )
  async function download() {
    setBusy(true)
    setError('')
    try {
      const blob = await load(),
        url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = `statement-${statements.find((item) => item.id === selected)?.language || 'preview'}.pdf`
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      setError(apiError(error, '题面下载失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="min-w-0 space-y-4 rounded-xl border bg-card p-4" aria-label="编译后的题面">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-medium">编译后的题面</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            对应这次检查的材料，可在发布前核对排版。
          </p>
        </div>
        {selected && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setSelected('')}
            aria-label="关闭题面预览"
          >
            <X />
          </Button>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        {statements.map((item) => (
          <Button
            key={item.id}
            variant={selected === item.id ? 'secondary' : 'outline'}
            onClick={() => {
              setSelected(item.id)
              setError('')
            }}
            aria-pressed={selected === item.id}
          >
            <Eye />
            {item.language} · PDF
          </Button>
        ))}
        {selected && (
          <Button variant="outline" onClick={() => void download()} loading={busy}>
            <Download />
            下载题面
          </Button>
        )}
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {selected && (
        <Suspense fallback={<p className="text-sm text-muted-foreground">正在加载预览…</p>}>
          <PdfStatement key={selected} load={load} />
        </Suspense>
      )}
    </section>
  )
}
