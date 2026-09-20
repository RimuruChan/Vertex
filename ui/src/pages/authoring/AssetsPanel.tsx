import { attachmentMarkdown } from './asset-links'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Image, FileText, File, Upload, Download, Trash2, Link2, Lock, Globe } from 'lucide-react'
import type { DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError, formatFileSize } from '@/lib/format'
import { Choice } from './FormFields'
import { entryLabel } from '@/lib/authoring-materials'

export default function AssetsPanel({
  problemId,
  copy,
  canEdit,
  onSaved,
  onBusy,
}: {
  problemId: string
  copy: DomainWorkingCopy
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onBusy: (busy: boolean) => void
}) {
  const api = useDomainAPI(),
    confirm = useConfirm(),
    upload = useRef<HTMLInputElement>(null)
  const [scope, setScope] = useState<'public' | 'private'>('public'),
    [selected, setSelected] = useState(''),
    [query, setQuery] = useState(''),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [image, setImage] = useState(''),
    [preview, setPreview] = useState(''),
    [statementId, setStatementId] = useState('')
  const all = copy.tree.entries.filter((e) => ['asset', 'resource'].includes(e.kind)),
    files = all.filter(
      (e) =>
        (scope === 'public'
          ? e.kind === 'asset' && e.attributes.visibility !== 'private'
          : e.kind === 'resource' || e.attributes.visibility === 'private') &&
        e.path.toLowerCase().includes(query.toLowerCase()),
    )
  const file = files.find((e) => e.id === selected),
    statements = copy.tree.entries.filter(
      (e) => e.kind === 'statement' && ['markdown', 'tex'].includes(e.attributes.format),
    )
  const statement = statements.find((e) => e.id === statementId) ?? statements[0]
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  const load = useCallback(
    (entry: DomainTreeEntry) =>
      api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256),
    [api, problemId],
  )
  useEffect(() => {
    let live = true,
      url = ''
    setImage('')
    setPreview('')
    if (!file) return
    if (/\.(png|jpe?g|webp|gif)$/i.test(file.path) && file.blob.bytes <= 8 * 1024 * 1024)
      void load(file)
        .then((blob) => {
          if (live) {
            url = URL.createObjectURL(blob)
            setImage(url)
          }
        })
        .catch(() => {
          if (live) setPreview('图片未能加载，可下载查看。')
        })
    else if (
      /\.(txt|md|tex|json|ya?ml|cpp|py|c|h)$/i.test(file.path) &&
      file.blob.bytes < 64 * 1024
    )
      void load(file)
        .then(async (blob) => {
          const content = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
          if (live) setPreview(content)
        })
        .catch(() => {})
    return () => {
      live = false
      if (url) URL.revokeObjectURL(url)
    }
  }, [file?.id, file?.blob.sha256, load])
  async function add(incoming: File[]) {
    if (!incoming.length) return
    setBusy(true)
    setError('')
    setNotice('')
    try {
      if (incoming.length > 100) throw new Error('每次最多上传 100 个文件')
      const entries = [...copy.tree.entries]
      const added: string[] = []
      const paths = new Set(entries.map((entry) => entry.path.toLowerCase()))
      for (const local of incoming) {
        if (local.size > 64 * 1024 * 1024) throw new Error(`${local.name} 超过 64 MiB`)
        const id = crypto.randomUUID()
        added.push(id)
        let path = `${scope === 'public' ? 'attachments' : 'resources'}/${local.name}`
        if (paths.has(path.toLowerCase()))
          path = `${scope === 'public' ? 'attachments' : 'resources'}/${id.slice(0, 8)}-${local.name}`
        paths.add(path.toLowerCase())
        entries.push({
          id,
          kind: scope === 'public' ? 'asset' : 'resource',
          path,
          attributes: {
            label: local.name,
            ...(scope === 'public' ? { visibility: 'public' } : {}),
          },
          blob: await api.postApiAuthoringProblemsIdBlobs(problemId, { file: local }),
        })
      }
      onSaved(
        await api.putApiAuthoringProblemsIdWorkingCopy(problemId, {
          etag: copy.etag,
          tree: { entries },
        }),
      )
      setNotice(`已添加 ${incoming.length} 个文件`)
      setSelected(added[0] ?? '')
    } catch (e) {
      setError(apiError(e, '上传失败'))
    } finally {
      setBusy(false)
    }
  }
  async function download() {
    if (!file) return
    try {
      const blob = await load(file),
        url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = file.attributes.label || file.path.split('/').pop()!
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(apiError(e, '下载失败'))
    }
  }
  async function remove() {
    if (
      !file ||
      !(await confirm({
        title: `移除 ${file.attributes.label || file.path.split('/').pop()}？`,
        description: '已发布版本不受影响。题面若仍引用此文件，请同时修改对应内容。',
        confirmLabel: '移除文件',
        destructive: true,
      }))
    )
      return
    setBusy(true)
    try {
      onSaved(
        await api.deleteApiAuthoringProblemsIdWorkingCopyEntriesEntryId(problemId, file.id, {
          etag: copy.etag,
        }),
      )
      setSelected('')
    } catch (e) {
      setError(apiError(e, '移除失败'))
    } finally {
      setBusy(false)
    }
  }
  async function insert() {
    if (!file || !statement) return
    setBusy(true)
    setError('')
    setNotice('')
    try {
      if (statement.blob.bytes > 1024 * 1024) throw new Error('题面过大，请在编辑器中手动插入链接')
      const source = await load(statement),
        text = await source.text()
      onSaved(
        await api.putApiAuthoringProblemsIdWorkingCopyEntriesEntryId(problemId, statement.id, {
          etag: copy.etag,
          entry: statement,
          text: text.trimEnd() + '\n\n' + attachmentMarkdown(statement, file) + '\n',
        }),
      )
      setNotice('已插入题面末尾，可回到题面调整位置。')
    } catch (e) {
      setError(apiError(e, '插入失败，题面未改变'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">图片与附件</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            管理题面图片、供参赛者下载的文件，以及内部参考资料。
          </p>
        </div>
        {canEdit && (
          <Button disabled={busy} onClick={() => upload.current?.click()}>
            <Upload />
            上传{scope === 'public' ? '附件' : '内部文件'}
          </Button>
        )}
      </header>
      <input
        ref={upload}
        type="file"
        className="hidden"
        multiple
        aria-label="选择附件"
        onChange={(e) => {
          void add(Array.from(e.target.files ?? []))
          e.target.value = ''
        }}
      />
      <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3">
        <div className="flex gap-1">
          {[
            ['public', '公开附件', Globe],
            ['private', '内部资料', Lock],
          ].map(([id, title, Icon]) => (
            <Button
              key={id as string}
              size="sm"
              variant={scope === id ? 'secondary' : 'ghost'}
              disabled={busy}
              onClick={() => {
                setScope(id as 'public' | 'private')
                setSelected('')
              }}
            >
              {typeof Icon !== 'string' && <Icon className="size-4" />}
              {title as string}
            </Button>
          ))}
        </div>
        <Input
          aria-label="搜索附件"
          className="max-w-56"
          value={query}
          placeholder="搜索文件…"
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {scope === 'public'
          ? '随题目发布后可供参赛者查看或下载。'
          : '仅用于出题协作，不会出现在公开题面和下载列表中。'}
      </p>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="text-sm text-primary">
          {notice}
        </p>
      )}
      <div className="grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div
          className="min-w-0"
          onDragOver={(e) => {
            if (canEdit) e.preventDefault()
          }}
          onDrop={(e) => {
            e.preventDefault()
            if (canEdit && !busy) void add(Array.from(e.dataTransfer.files))
          }}
        >
          {files.length ? (
            <div className="grid gap-3 sm:grid-cols-2 2xl:grid-cols-3">
              {files.map((entry) => {
                const Icon = /\.(png|jpe?g|webp|gif)$/i.test(entry.path)
                  ? Image
                  : /\.(txt|md|tex|pdf)$/i.test(entry.path)
                    ? FileText
                    : File
                return (
                  <button
                    key={entry.id}
                    disabled={busy}
                    onClick={() => setSelected(entry.id)}
                    className={`min-w-0 rounded-xl border p-4 text-left transition-colors ${file?.id === entry.id ? 'border-primary bg-primary/5 ring-1 ring-primary/15' : 'bg-card hover:border-primary/40'}`}
                  >
                    <Icon className="mb-4 size-7 text-muted-foreground" />
                    <span className="block truncate text-sm font-medium">
                      {entry.attributes.label || entry.path.split('/').pop()}
                    </span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {formatFileSize(entry.blob.bytes)}
                    </span>
                  </button>
                )
              })}
            </div>
          ) : (
            <div className="rounded-xl border-2 border-dashed bg-muted/15 px-6 py-16 text-center">
              <Upload className="mx-auto mb-3 size-7 text-muted-foreground" />
              <p className="text-sm font-medium">{query ? '没有匹配的文件' : '把文件拖到这里'}</p>
              <p className="mt-2 text-xs text-muted-foreground">
                {scope === 'public'
                  ? '图片上传后可以直接插入题面。'
                  : '保存出题过程中的说明、参考资料或辅助文件。'}
              </p>
            </div>
          )}
        </div>
        <aside className="min-w-0 rounded-xl border bg-card p-4 xl:sticky xl:top-24">
          {file ? (
            <div className="space-y-4">
              <h3 className="break-words text-sm font-semibold">
                {file.attributes.label || file.path.split('/').pop()}
              </h3>
              {image ? (
                <img
                  src={image}
                  alt={file.attributes.label || '附件预览'}
                  className="max-h-64 w-full rounded-lg border bg-white object-contain"
                />
              ) : preview ? (
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/35 p-3 text-xs leading-6">
                  {preview}
                </pre>
              ) : (
                <div className="flex h-36 items-center justify-center rounded-lg bg-muted/30">
                  <File className="size-10 text-muted-foreground" />
                </div>
              )}
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={() => void download()}
                  disabled={busy}
                >
                  <Download />
                  下载
                </Button>
                {canEdit && (
                  <Button
                    variant="outline"
                    size="icon"
                    aria-label="移除附件"
                    disabled={busy}
                    onClick={() => void remove()}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                )}
              </div>
              {scope === 'public' && canEdit && (
                <div className="space-y-3 border-t pt-4">
                  {statements.length > 1 && (
                    <Choice
                      label="插入到题面"
                      value={statement?.id ?? ''}
                      onChange={setStatementId}
                      options={statements.map((s) => [s.id, entryLabel(s)])}
                    />
                  )}
                  <Button
                    variant="outline"
                    className="w-full"
                    disabled={busy || !statement}
                    onClick={() => void insert()}
                  >
                    <Link2 />
                    插入题面
                  </Button>
                  {!statement && (
                    <p className="text-xs text-muted-foreground">
                      添加 Markdown 或 TeX 题面后可直接插入。
                    </p>
                  )}
                </div>
              )}
              <details className="border-t pt-3 text-xs text-muted-foreground">
                <summary className="cursor-pointer">文件位置</summary>
                <p className="mt-2 break-all">{file.path}</p>
              </details>
            </div>
          ) : (
            <div className="py-12 text-center">
              <Image className="mx-auto mb-3 size-7 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">选择文件预览</p>
              <p className="mt-2 text-xs text-muted-foreground">
                下载、插入题面与移除操作都在这里。
              </p>
            </div>
          )}
        </aside>
      </div>
    </div>
  )
}
