import { useEffect, useMemo, useState } from 'react'
import type { DomainTreeEntry } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import MdRenderer, { markdownImages, markdownLinks } from '@/components/MdRenderer'
import { apiError } from '@/lib/format'
import { publishedAssetPath } from '@/lib/published-files'
import { previewSourceLines } from '@/lib/statement-scroll'

export default function StatementPreview({
  etag,
  revision,
  problemId,
  entry,
  entries,
  content,
}: {
  etag: string
  revision?: number
  problemId: string
  entry: DomainTreeEntry
  entries: DomainTreeEntry[]
  content: string
}) {
  const api = useDomainAPI(),
    [images, setImages] = useState<Record<string, string>>({}),
    [error, setError] = useState('')
  const [rendered, setRendered] = useState<{ content: string; warnings: string[] }>()
  const [previewError, setPreviewError] = useState('')
  useEffect(() => {
    let live = true
    setRendered(undefined)
    setPreviewError('')
    const timer = setTimeout(() => {
      void api
        .postApiAuthoringProblemsIdStatementPreview(problemId, {
          etag,
          revision: revision ?? 0,
          entryId: entry.id,
          content,
        })
        .then((result) => {
          if (live) setRendered(result)
        })
        .catch((e) => {
          if (live) setPreviewError(apiError(e, '样例预览暂不可用'))
        })
    }, 350)
    return () => {
      live = false
      clearTimeout(timer)
    }
  }, [api, problemId, etag, revision, entry.id, content])
  const sources = useMemo(() => markdownImages(content), [content])
  const displayContent = rendered?.content || content || '*题面预览会显示在这里。*'
  const sourceLineMap = useMemo(
    () => previewSourceLines(content, displayContent),
    [content, displayContent],
  )
  const links = useMemo(() => markdownLinks(content), [content])
  const attachments = new Map(
    links.map((source) => [
      source,
      entries.find(
        (file) =>
          file.kind === 'asset' &&
          file.attributes.visibility !== 'private' &&
          file.path === publishedAssetPath(entry.path, source),
      ),
    ]),
  )
  const resources = sources.map((source) => ({
    source,
    file: entries.find(
      (file) =>
        file.kind === 'asset' &&
        file.attributes.visibility !== 'private' &&
        file.path === publishedAssetPath(entry.path, source),
    ),
  }))
  const signature = JSON.stringify(resources.map(({ source, file }) => [source, file?.blob.sha256]))
  useEffect(() => {
    let live = true
    const urls: string[] = []
    setImages({})
    setError('')
    let total = 0
    for (const { source, file } of resources) {
      if (!file || !/\.(png|jpe?g|gif|webp)$/i.test(file.path)) {
        setError('部分图片未关联公开附件，请在“图片与附件”中上传并插入。')
        continue
      }
      total += file.blob.bytes
      if (file.blob.bytes > 8 * 1024 * 1024 || total > 32 * 1024 * 1024) {
        setError('图片过大，请在附件页下载查看。')
        continue
      }
      void api
        .getApiAuthoringProblemsIdBlobsDigest(problemId, file.blob.sha256)
        .then((blob) => {
          if (!live) return
          const url = URL.createObjectURL(blob)
          urls.push(url)
          setImages((current) => ({ ...current, [source]: url }))
        })
        .catch(() => {
          if (live) setError('部分图片未能加载，请重新打开预览。')
        })
    }
    return () => {
      live = false
      urls.forEach((url) => URL.revokeObjectURL(url))
    }
  }, [api, problemId, signature])
  return (
    <div
      onClick={async (event) => {
        const anchor = (event.target as HTMLElement).closest('a')
        if (!anchor) return
        const href = anchor.getAttribute('href') ?? ''
        if (/^(https?:|mailto:|#)/i.test(href)) return
        event.preventDefault()
        const file = attachments.get(href)
        if (!file) {
          setError('此链接未关联公开附件，请通过“图片与附件”重新插入。')
          return
        }
        try {
          const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, file.blob.sha256)
          const url = URL.createObjectURL(blob),
            download = document.createElement('a')
          download.href = url
          download.download = file.attributes.label || file.path.split('/').pop() || 'attachment'
          download.click()
          setTimeout(() => URL.revokeObjectURL(url), 1000)
        } catch (e) {
          setError(apiError(e, '下载附件失败'))
        }
      }}
    >
      {[previewError, ...(rendered?.warnings ?? [])].filter(Boolean).map((message, index) => (
        <p
          key={index}
          role="status"
          className="mb-3 rounded-lg border bg-muted/20 p-3 text-xs text-muted-foreground"
        >
          {message}
        </p>
      ))}
      {error && (
        <p role="status" className="mb-4 rounded-lg bg-muted/40 p-3 text-xs text-muted-foreground">
          {error}
        </p>
      )}
      <MdRenderer
        content={displayContent}
        sourceLineMap={sourceLineMap}
        media={{
          images: Object.fromEntries(sources.map((source) => [source, images[source] ?? ''])),
          links: {},
        }}
      />
    </div>
  )
}
