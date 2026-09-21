import { useEffect, useRef, useState } from 'react'
import { getDocument, GlobalWorkerOptions, type PDFDocumentProxy } from 'pdfjs-dist'
import workerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { ChevronLeft, ChevronRight, LoaderCircle, ZoomIn, ZoomOut } from 'lucide-react'
import { Button } from '@/components/ui/button'

GlobalWorkerOptions.workerSrc = workerURL

// Render document pages only. No annotation layer, JavaScript actions, forms or
// external document URLs are executed by this viewer.
export default function PdfStatement({ load }: { load: () => Promise<Blob> }) {
  const [pdf, setPdf] = useState<PDFDocumentProxy>(),
    [page, setPage] = useState(1),
    [width, setWidth] = useState(600),
    [zoom, setZoom] = useState(1)
  const [error, setError] = useState(''),
    [rendering, setRendering] = useState(true)
  const [pageText, setPageText] = useState('')
  const container = useRef<HTMLDivElement>(null),
    canvas = useRef<HTMLCanvasElement>(null)
  const loader = useRef<ReturnType<typeof getDocument> | undefined>(undefined)
  useEffect(() => {
    const element = container.current
    if (!element) return
    const observer = new ResizeObserver((entries) =>
      setWidth(Math.max(1, entries[0].contentRect.width)),
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  useEffect(() => {
    let active = true
    let task: ReturnType<typeof getDocument> | undefined
    const deadline = window.setTimeout(() => {
      if (!active) return
      active = false
      void task?.destroy()
      setRendering(false)
      setError('PDF 加载超时，可下载题面文件阅读。')
    }, 30000)
    setPdf(undefined)
    setError('')
    setPage(1)
    setZoom(1)
    void (async () => {
      const file = await load()
      if (!active) return
      if (file.size > 32 * 1024 * 1024) throw new Error('PDF 超过在线预览限制，请下载阅读。')
      task = getDocument({
        data: new Uint8Array(await file.arrayBuffer()),
        enableXfa: false,
        useWorkerFetch: false,
        useWasm: false,
        maxImageSize: 16_000_000,
        canvasMaxAreaInBytes: 64_000_000,
        disableAutoFetch: true,
        stopAtErrors: true,
      })
      loader.current = task
      const document = await task.promise
      if (active) setPdf(document)
    })()
      .catch(() => {
        if (active) setError('PDF 预览失败，可下载题面文件阅读。')
      })
      .finally(() => window.clearTimeout(deadline))
    return () => {
      active = false
      window.clearTimeout(deadline)
      void task?.destroy()
      if (loader.current === task) loader.current = undefined
    }
  }, [load])
  useEffect(() => {
    if (!pdf || !canvas.current) return
    const element = canvas.current
    let active = true
    let task: ReturnType<Awaited<ReturnType<PDFDocumentProxy['getPage']>>['render']> | undefined
    const deadline = window.setTimeout(() => {
      if (!active) return
      active = false
      task?.cancel()
      void loader.current?.destroy()
      setPdf(undefined)
      setRendering(false)
      setError('PDF 页面预览超时，可下载题面文件阅读。')
    }, 20000)
    setRendering(true)
    setError('')
    setPageText('')
    void (async () => {
      const documentPage = await pdf.getPage(page)
      if (!active) return
      const original = documentPage.getViewport({ scale: 1 })
      const scale = Math.min(
        ((width * zoom) / original.width) * Math.min(devicePixelRatio || 1, 2),
        Math.sqrt(12_000_000 / (original.width * original.height)),
        4,
      )
      const viewport = documentPage.getViewport({ scale })
      element.width = Math.ceil(viewport.width)
      element.height = Math.ceil(viewport.height)
      task = documentPage.render({ canvas: element, viewport, annotationMode: 0 })
      await task.promise
      window.clearTimeout(deadline)
      if (active) {
        setRendering(false)
        const text = await documentPage.getTextContent()
        if (active) {
          let preview = ''
          for (const item of text.items) {
            if ('str' in item)
              preview += (item.str + (item.hasEOL ? '\n' : ' ')).slice(0, 200000 - preview.length)
            if (preview.length >= 200000) break
          }
          setPageText(preview)
        }
      }
    })()
      .catch(() => {
        if (active) {
          setRendering(false)
          setError('本页无法预览，可下载题面阅读。')
        }
      })
      .finally(() => window.clearTimeout(deadline))
    return () => {
      active = false
      window.clearTimeout(deadline)
      task?.cancel()
    }
  }, [pdf, page, width, zoom])
  return (
    <div className="space-y-3" ref={container}>
      {pdf && (
        <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
          <span className="tabular-nums text-muted-foreground">
            第 {page} / {pdf.numPages} 页
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={zoom <= 1}
              onClick={() => setZoom(zoom - 0.5)}
              aria-label="缩小 PDF"
            >
              <ZoomOut />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={zoom >= 3}
              onClick={() => setZoom(zoom + 0.5)}
              aria-label="放大 PDF"
            >
              <ZoomIn />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
              aria-label="PDF 上一页"
            >
              <ChevronLeft />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= pdf.numPages}
              onClick={() => setPage(page + 1)}
              aria-label="PDF 下一页"
            >
              <ChevronRight />
            </Button>
          </div>
        </div>
      )}
      {error && (
        <p
          role="alert"
          className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      <div
        hidden={Boolean(error)}
        className="relative min-h-48 overflow-auto rounded-lg border bg-white"
      >
        {(!pdf || rendering) && (
          <div
            role="status"
            className="absolute inset-0 flex items-center justify-center gap-2 bg-background/70 text-sm text-muted-foreground"
          >
            <LoaderCircle className="size-4 animate-spin" />
            正在显示题面…
          </div>
        )}
        <canvas
          key={`${page}:${width}:${zoom}`}
          ref={canvas}
          className="block h-auto max-w-none"
          style={{ width: width * zoom }}
          aria-label={`PDF 题面，第 ${page} 页`}
        />
      </div>
      {pageText && (
        <details className="rounded-lg border p-3 text-sm">
          <summary className="cursor-pointer text-muted-foreground">本页文字内容</summary>
          <pre className="mt-3 max-h-80 overflow-auto whitespace-pre-wrap font-sans leading-6">
            {pageText}
          </pre>
        </details>
      )}
    </div>
  )
}
