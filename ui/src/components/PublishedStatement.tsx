import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { Download } from 'lucide-react'
import type { DtoPublishedFileResponse } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import MdRenderer, { markdownImages, markdownLinks } from '@/components/MdRenderer'
import { apiError } from '@/lib/format'
import { publishedAssetPath, publishedSamples } from '@/lib/published-files'

const PdfStatement = lazy(() => import('./PdfStatement'))
const emptyFiles: DtoPublishedFileResponse[] = []
export default function PublishedStatement({
  problemId,
  contestId,
  label,
  version,
  content,
  files = emptyFiles,
}: {
  problemId: string
  contestId?: string
  label?: string
  version: number
  content: string
  files?: DtoPublishedFileResponse[]
}) {
  const api = useDomainAPI(),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(''),
    [images, setImages] = useState<Record<string, string>>({})
  const statement = files.find((file) => file.purpose === 'statement')
  const attachments = files.filter((file) => file.purpose === 'asset')
  const samples = publishedSamples(files)
  const embeddedSamples = files.filter((file) => file.sampleIndex && file.embedded)
  const load = useCallback(
    (fileId: string) =>
      contestId
        ? api.getApiContestsIdProblemsProblemIdFilesFileId(contestId, label || problemId, fileId, {
            version,
          })
        : api.getApiProblemsIdFilesFileId(problemId, fileId, { version }),
    [api, contestId, label, problemId, version],
  )
  const statementId = statement?.id
  const loadPDF = useCallback(() => load(statementId!), [load, statementId])
  const referenced = useMemo(
    () =>
      markdownImages(content).flatMap((source) => {
        const target = publishedAssetPath(statement?.path || 'statement/problem.md', source)
        const file = files.find(
          (file) =>
            file.purpose === 'asset' &&
            file.path === target &&
            /^image\/(png|jpeg|gif|webp)$/.test(file.mediaType),
        )
        return file ? [{ source, file }] : []
      }),
    [content, files, statement?.path],
  )
  const links = useMemo(
    () =>
      Object.fromEntries(
        markdownLinks(content).flatMap((source) => {
          const target = publishedAssetPath(statement?.path || 'statement/problem.md', source)
          const file = files.find((file) => file.purpose === 'asset' && file.path === target)
          return file ? [[source, '#file-' + file.id]] : []
        }),
      ),
    [content, files, statement?.path],
  )
  const media = useMemo(() => ({ images, links }), [images, links])
  useEffect(() => {
    let active = true
    const urls: string[] = []
    setImages(Object.fromEntries(referenced.map(({ source }) => [source, ''])))
    let imageBytes = 0
    for (const { source, file } of referenced) {
      imageBytes += file.size
      if (file.size > 8 * 1024 * 1024 || imageBytes > 32 * 1024 * 1024) {
        setError('部分图片超过在线预览大小，请从附件下载。')
        continue
      }
      void load(file.id)
        .then((blob) => {
          if (!active) return
          const url = URL.createObjectURL(blob)
          urls.push(url)
          setImages((current) => ({ ...current, [source]: url }))
        })
        .catch(() => {
          if (active) setError('部分题面图片加载失败，请刷新重试。')
        })
    }
    return () => {
      active = false
      urls.forEach((url) => URL.revokeObjectURL(url))
    }
  }, [load, referenced])
  async function download(file: DtoPublishedFileResponse) {
    setBusy(file.id)
    setError('')
    try {
      const blob = await load(file.id),
        url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = file.name
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      setError(apiError(error, '文件下载失败'))
    } finally {
      setBusy('')
    }
  }
  const fileButton = (file: DtoPublishedFileResponse, text?: string) => (
    <Button
      variant="outline"
      size="sm"
      loading={busy === file.id}
      disabled={Boolean(busy)}
      onClick={() => void download(file)}
    >
      <Download />
      {text || file.name}
    </Button>
  )
  return (
    <div className="space-y-5">
      {statement?.mediaType === 'application/pdf' ? (
        <>
          <div className="flex justify-end">{fileButton(statement, '下载 PDF 题面')}</div>
          <Suspense fallback={<p className="py-8 text-sm text-muted-foreground">正在加载题面…</p>}>
            <PdfStatement load={loadPDF} />
          </Suspense>
        </>
      ) : (
        <MdRenderer content={content} media={media} className="problem-statement" />
      )}
      {samples.length > 0 && (
        <section aria-label="样例" className="space-y-4">
          <h2 className="text-lg font-semibold">样例</h2>
          {samples.map((sample) => (
            <div
              key={sample.index}
              id={`sample-${sample.index}`}
              className="space-y-2 scroll-mt-24"
            >
              <h3 className="text-sm font-medium">样例 {sample.index}</h3>
              <div className="grid gap-3 md:grid-cols-2">
                {(
                  [
                    ['输入', sample.input],
                    ['输出', sample.answer],
                  ] as const
                ).map(
                  ([title, file]) =>
                    file && (
                      <div key={file.id} className="min-w-0 overflow-hidden rounded-lg border">
                        <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/30 px-3 py-2">
                          <span className="text-sm">{title}</span>
                          {fileButton(file, '下载')}
                        </div>
                        {file.binary ? (
                          <p className="p-3 text-sm text-muted-foreground">
                            二进制数据，请下载查看（{file.size.toLocaleString()} 字节）。
                          </p>
                        ) : (
                          <pre className="max-h-72 overflow-auto whitespace-pre p-3 font-mono text-sm">
                            {file.preview || ''}
                          </pre>
                        )}
                        {file.truncated && (
                          <p className="border-t px-3 py-2 text-xs text-muted-foreground">
                            仅显示部分内容；下载可获得完整数据（{file.size.toLocaleString()}{' '}
                            字节）。
                          </p>
                        )}
                      </div>
                    ),
                )}
              </div>
            </div>
          ))}
        </section>
      )}
      {embeddedSamples.length > 0 && (
        <section aria-label="样例下载" className="space-y-2">
          <h2 className="text-base font-semibold">样例下载</h2>
          <div className="flex flex-wrap gap-2">
            {embeddedSamples.map((file) => (
              <div key={file.id}>
                {fileButton(
                  file,
                  `样例 ${file.sampleIndex} · ${file.purpose === 'sample-input' ? '输入' : '输出'}`,
                )}
              </div>
            ))}
          </div>
        </section>
      )}
      {attachments.length > 0 && (
        <section aria-label="题目附件" className="space-y-2">
          <h2 className="text-base font-semibold">附件</h2>
          <div className="flex flex-wrap gap-2">
            {attachments.map((file) => (
              <div key={file.id} id={'file-' + file.id}>
                {fileButton(file)}
              </div>
            ))}
          </div>
        </section>
      )}
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
    </div>
  )
}
