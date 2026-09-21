import { useEffect, useState } from 'react'
import { Image, File } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type { DomainTreeEntry } from '@/generated/api/model'
import { entryLabel } from '@/lib/authoring-materials'

export default function AttachmentPreview({
  problemId,
  file,
}: {
  problemId: string
  file?: DomainTreeEntry
}) {
  const api = useDomainAPI(),
    [image, setImage] = useState(''),
    [message, setMessage] = useState('')
  useEffect(() => {
    let live = true,
      url = ''
    setImage('')
    setMessage('')
    if (!file || !/\.(png|jpe?g|gif|webp)$/i.test(file.path) || file.blob.bytes > 8 * 1024 * 1024)
      return
    setMessage('正在加载预览…')
    void api
      .getApiAuthoringProblemsIdBlobsDigest(problemId, file.blob.sha256)
      .then((blob) => {
        if (!live) return
        url = URL.createObjectURL(blob)
        setImage(url)
        setMessage('')
      })
      .catch(() => {
        if (live) setMessage('暂时无法预览，可继续插入附件。')
      })
    return () => {
      live = false
      if (url) URL.revokeObjectURL(url)
    }
  }, [api, problemId, file?.blob.sha256])
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 overflow-hidden rounded-lg border bg-muted/20 p-4 text-center">
      {image ? (
        <img
          src={image}
          alt={file ? entryLabel(file) : ''}
          className="max-h-48 max-w-full object-contain"
        />
      ) : file ? (
        <File className="size-8 text-muted-foreground" />
      ) : (
        <Image className="size-8 text-muted-foreground" />
      )}
      <p className="max-w-full break-words text-xs text-muted-foreground">
        {message || (file ? entryLabel(file) : '选择附件预览')}
      </p>
    </div>
  )
}
