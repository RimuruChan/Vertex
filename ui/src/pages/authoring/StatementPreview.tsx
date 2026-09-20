import { useEffect, useMemo, useState } from 'react'
import type { DomainTreeEntry } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import MdRenderer, { markdownImages } from '@/components/MdRenderer'
import { publishedAssetPath } from '@/lib/published-files'

export default function StatementPreview({
  problemId,
  entry,
  entries,
  content,
}: {
  problemId: string
  entry: DomainTreeEntry
  entries: DomainTreeEntry[]
  content: string
}) {
  const api = useDomainAPI(),
    [images, setImages] = useState<Record<string, string>>({}),
    [error, setError] = useState('')
  const sources = useMemo(() => markdownImages(content), [content])
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
    <>
      {error && (
        <p role="status" className="mb-4 rounded-lg bg-muted/40 p-3 text-xs text-muted-foreground">
          {error}
        </p>
      )}
      <MdRenderer
        content={content || '*题面预览会显示在这里。*'}
        media={{
          images: Object.fromEntries(sources.map((source) => [source, images[source] ?? ''])),
          links: {},
        }}
      />
    </>
  )
}
