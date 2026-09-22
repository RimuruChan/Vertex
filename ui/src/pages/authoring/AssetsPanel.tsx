import { availableLocation } from './material-locations'
import { attachmentMarkdown } from './asset-links'
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Image,
  FileText,
  File,
  Upload,
  Download,
  Trash2,
  Link2,
  Lock,
  Globe,
  Search,
  X,
  Copy,
  CheckCircle2,
  CircleAlert,
  Loader2,
  RefreshCw,
  ArrowRight,
  FolderArchive,
} from 'lucide-react'
import type { DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError, formatFileSize } from '@/lib/format'
import { Choice } from './FormFields'
import { entryLabel } from '@/lib/authoring-materials'
import { Link } from '@/domain/navigation'
import {
  assetScope,
  assetType,
  assetUploadErrors,
  assetPreviewKind,
  type AssetType,
} from './asset-management'

const typeLabels: Record<AssetType, string> = {
  all: '全部类型',
  image: '图片',
  document: '文本与文档',
  archive: '压缩包',
  other: '其他文件',
}

export default function AssetsPanel({
  problemId,
  copy,
  canEdit,
  onSaved,
  onBusy,
  initialEntryId,
  initialEntryRequest,
}: {
  problemId: string
  copy: DomainWorkingCopy
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onBusy: (busy: boolean) => void
  initialEntryId?: string
  initialEntryRequest?: number
}) {
  const api = useDomainAPI(),
    confirm = useConfirm(),
    upload = useRef<HTMLInputElement>(null),
    details = useRef<HTMLElement>(null),
    actionLock = useRef(false),
    clipboardLock = useRef(false),
    downloadLock = useRef(false),
    handledEntry = useRef<{ id: string; request?: number } | undefined>(undefined)
  const [scope, setScope] = useState<'public' | 'private'>('public'),
    [selected, setSelected] = useState(''),
    [query, setQuery] = useState(''),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [notice, setNotice] = useState(''),
    [statementId, setStatementId] = useState('')
  const [type, setType] = useState<AssetType>('all'),
    [previewAttempt, setPreviewAttempt] = useState(0),
    [downloading, setDownloading] = useState(false),
    [copying, setCopying] = useState(false),
    [insertedStatement, setInsertedStatement] = useState('')
  const [previewResult, setPreviewResult] = useState<{
    key: string
    state: 'loading' | 'ready' | 'error' | 'unavailable'
    image?: string
    text?: string
    error?: string
  }>()
  const [uploadProgress, setUploadProgress] = useState<{
    phase: 'uploading' | 'saving' | 'saved' | 'failed'
    files: { name: string; state: 'pending' | 'uploading' | 'uploaded' | 'failed' }[]
  }>()
  const [addedFiles, setAddedFiles] = useState<{ ids: string[]; scope: 'public' | 'private' }>()
  const all = copy.tree.entries.filter((e) => ['asset', 'resource'].includes(e.kind)),
    scoped = all.filter((entry) => assetScope(entry) === scope),
    files = all.filter(
      (e) =>
        assetScope(e) === scope &&
        (type === 'all' || assetType(e.path) === type) &&
        entryLabel(e).toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
    )
  const file = scoped.find((e) => e.id === selected),
    statements = copy.tree.entries.filter(
      (e) => e.kind === 'statement' && ['markdown', 'tex'].includes(e.attributes.format),
    )
  const statement = statements.find((e) => e.id === statementId) ?? statements[0]
  const filtered = Boolean(query.trim() || type !== 'all')
  const previewKey = file ? `${file.id}:${file.blob.sha256}:${previewAttempt}` : ''
  const preview = previewResult?.key === previewKey ? previewResult : undefined
  const previewKind = file ? assetPreviewKind(file) : 'unavailable'
  let reference = '',
    referenceError = ''
  if (file && assetScope(file) === 'public' && statement) {
    try {
      reference = attachmentMarkdown(statement, file)
    } catch (error) {
      referenceError = apiError(error, '此题面不支持引用该文件')
    }
  }
  function clearFilters() {
    setQuery('')
    setType('all')
  }
  function showAdded() {
    if (!addedFiles) return
    setScope(addedFiles.scope)
    clearFilters()
    selectFile(addedFiles.ids[0] ?? '')
  }
  function selectFile(id: string) {
    setSelected(id)
    requestAnimationFrame(() => {
      if (window.innerWidth < 1024)
        details.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      details.current?.focus({ preventScroll: true })
    })
  }
  useEffect(() => {
    if (!initialEntryId) {
      handledEntry.current = undefined
      return
    }
    if (
      busy ||
      (handledEntry.current?.id === initialEntryId &&
        handledEntry.current.request === initialEntryRequest)
    )
      return
    handledEntry.current = { id: initialEntryId, request: initialEntryRequest }
    const target = copy.tree.entries.find(
      (entry) => entry.id === initialEntryId && ['asset', 'resource'].includes(entry.kind),
    )
    if (!target) {
      setError('没有找到对应的附件，文件可能已经移除。')
      return
    }
    setScope(assetScope(target))
    setSelected(target.id)
    setQuery('')
    setType('all')
    setError('')
    requestAnimationFrame(() => {
      if (window.innerWidth < 1024)
        details.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }, [initialEntryId, initialEntryRequest, copy.tree.entries, busy])
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
    if (!file) return
    const kind = assetPreviewKind(file)
    setPreviewResult({ key: previewKey, state: kind === 'unavailable' ? 'unavailable' : 'loading' })
    if (kind === 'image')
      void load(file)
        .then((blob) => {
          if (live) {
            url = URL.createObjectURL(blob)
            setPreviewResult({ key: previewKey, state: 'loading', image: url })
          }
        })
        .catch((error) => {
          if (live)
            setPreviewResult({
              key: previewKey,
              state: 'error',
              error: apiError(error, '图片预览加载失败'),
            })
        })
    else if (kind === 'text')
      void load(file)
        .then(async (blob) => {
          const content = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
          if (content.includes('\0')) throw new Error('文件包含二进制内容，请下载原文件查看。')
          if (live) setPreviewResult({ key: previewKey, state: 'ready', text: content })
        })
        .catch((error) => {
          if (live)
            setPreviewResult({
              key: previewKey,
              state: 'error',
              error: apiError(error, '文本预览失败，文件可能不是 UTF-8 文本'),
            })
        })
    return () => {
      live = false
      if (url) URL.revokeObjectURL(url)
    }
  }, [file?.id, file?.blob.sha256, previewKey, load])
  async function add(incoming: File[]) {
    if (!incoming.length || !canEdit || busy || actionLock.current) return
    setError('')
    setNotice('')
    setInsertedStatement('')
    setAddedFiles(undefined)
    const validation = assetUploadErrors(incoming)
    if (validation.length) {
      setError(validation.join('；'))
      setUploadProgress(undefined)
      return
    }
    actionLock.current = true
    setBusy(true)
    setUploadProgress({
      phase: 'uploading',
      files: incoming.map((file) => ({ name: file.name, state: 'pending' })),
    })
    let activeIndex = -1
    try {
      const entries = [...copy.tree.entries]
      const added: string[] = []
      for (const [index, local] of incoming.entries()) {
        activeIndex = index
        setUploadProgress(
          (current) =>
            current && {
              ...current,
              files: current.files.map((file, i) =>
                i === index ? { ...file, state: 'uploading' } : file,
              ),
            },
        )
        const id = crypto.randomUUID()
        added.push(id)
        const suffix = local.name.match(/\.[a-zA-Z0-9]{1,12}$/)?.[0].toLowerCase() ?? ''
        const path = availableLocation(
          `${scope === 'public' ? 'attachments' : 'resources'}/${id}${suffix}`,
          entries,
          id,
        )
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
        setUploadProgress(
          (current) =>
            current && {
              ...current,
              files: current.files.map((file, i) =>
                i === index ? { ...file, state: 'uploaded' } : file,
              ),
            },
        )
      }
      activeIndex = -1
      setUploadProgress((current) => current && { ...current, phase: 'saving' })
      onSaved(
        await api.putApiAuthoringProblemsIdWorkingCopy(problemId, {
          etag: copy.etag,
          tree: { entries },
        }),
      )
      setNotice(
        `已将 ${incoming.length} 个${scope === 'public' ? '公开附件' : '内部文件'}保存到个人草稿。`,
      )
      setUploadProgress((current) => current && { ...current, phase: 'saved' })
      setAddedFiles({ ids: added, scope })
      clearFilters()
      setSelected(added[0] ?? '')
    } catch (e) {
      setUploadProgress(
        (current) =>
          current && {
            ...current,
            phase: 'failed',
            files: current.files.map((file, index) =>
              index === activeIndex ? { ...file, state: 'failed' } : file,
            ),
          },
      )
      setError(apiError(e, '未能完成添加，请确认草稿状态后重试'))
    } finally {
      actionLock.current = false
      setBusy(false)
    }
  }
  async function download() {
    if (!file || busy || downloadLock.current) return
    downloadLock.current = true
    setDownloading(true)
    setError('')
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
    } finally {
      downloadLock.current = false
      setDownloading(false)
    }
  }
  async function copyText(text: string, label: string) {
    if (!text || busy || clipboardLock.current) return
    clipboardLock.current = true
    setCopying(true)
    setError('')
    setNotice('')
    try {
      await navigator.clipboard.writeText(text)
      setNotice(`已复制${label}。`)
    } catch {
      setError('复制失败，浏览器未允许访问剪贴板。可选中下方引用或路径手动复制。')
    } finally {
      clipboardLock.current = false
      setCopying(false)
    }
  }
  async function remove() {
    if (!file || !canEdit || busy || actionLock.current) return
    actionLock.current = true
    try {
      if (
        !(await confirm({
          title: `移除 ${file.attributes.label || file.path.split('/').pop()}？`,
          description: '已发布版本不受影响。题面若仍引用此文件，请同时修改对应内容。',
          confirmLabel: '移除文件',
          destructive: true,
        }))
      )
        return
      setBusy(true)
      setError('')
      setNotice('')
      onSaved(
        await api.deleteApiAuthoringProblemsIdWorkingCopyEntriesEntryId(problemId, file.id, {
          etag: copy.etag,
        }),
      )
      setSelected('')
      setNotice('文件已从个人草稿移除。')
      setAddedFiles(undefined)
      setInsertedStatement('')
    } catch (e) {
      setError(apiError(e, '移除失败'))
    } finally {
      actionLock.current = false
      setBusy(false)
    }
  }
  async function insert() {
    if (
      !file ||
      !statement ||
      !reference ||
      !canEdit ||
      busy ||
      actionLock.current ||
      assetScope(file) !== 'public'
    )
      return
    actionLock.current = true
    setBusy(true)
    setError('')
    setNotice('')
    setInsertedStatement('')
    try {
      if (statement.blob.bytes > 1024 * 1024) throw new Error('题面过大，请在编辑器中手动插入链接')
      const source = await load(statement),
        text = await source.text()
      onSaved(
        await api.putApiAuthoringProblemsIdWorkingCopyEntriesEntryId(problemId, statement.id, {
          etag: copy.etag,
          entry: statement,
          text: text.trimEnd() + '\n\n' + reference + '\n',
        }),
      )
      setNotice('已插入题面末尾，可回到题面调整位置。')
      setInsertedStatement(statement.id)
    } catch (e) {
      setError(apiError(e, '插入失败，题面未改变'))
    } finally {
      actionLock.current = false
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
        disabled={!canEdit || busy}
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
              aria-pressed={scope === id}
              disabled={busy}
              onClick={() => {
                setScope(id as 'public' | 'private')
                setSelected('')
              }}
            >
              {typeof Icon !== 'string' && <Icon className="size-4" />}
              {title as string}
              <span className="ml-1 text-xs tabular-nums text-muted-foreground">
                {all.filter((entry) => assetScope(entry) === id).length}
              </span>
            </Button>
          ))}
        </div>
      </div>
      <div className="space-y-3 rounded-xl border bg-card p-3 sm:p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="relative w-full sm:max-w-sm">
            <Search className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" />
            <Input
              aria-label="搜索附件"
              className="pl-9 pr-9"
              value={query}
              disabled={busy}
              placeholder="按文件名称搜索…"
              onChange={(e) => setQuery(e.target.value)}
            />
            {!!query && (
              <button
                type="button"
                disabled={busy}
                aria-label="清除附件搜索"
                className="absolute right-2 top-2 rounded p-1 text-muted-foreground hover:bg-muted"
                onClick={() => setQuery('')}
              >
                <X className="size-4" />
              </button>
            )}
          </div>
          <span role="status" className="text-xs text-muted-foreground">
            显示 {files.length} / {scoped.length} 个{scope === 'public' ? '公开附件' : '内部文件'}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-1.5" aria-label="附件类型筛选">
          {(Object.keys(typeLabels) as AssetType[]).map((value) => (
            <Button
              key={value}
              variant={type === value ? 'secondary' : 'ghost'}
              size="sm"
              disabled={busy}
              aria-pressed={type === value}
              onClick={() => setType(value)}
            >
              {typeLabels[value]}
              <span className="ml-1 text-xs tabular-nums text-muted-foreground">
                {value === 'all'
                  ? scoped.length
                  : scoped.filter((entry) => assetType(entry.path) === value).length}
              </span>
            </Button>
          ))}
          {filtered && (
            <Button variant="ghost" size="sm" disabled={busy} onClick={clearFilters}>
              <X />
              清除筛选
            </Button>
          )}
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {scope === 'public'
          ? '随题目发布后可供参赛者查看或下载。'
          : '仅用于出题协作，不会出现在公开题面和下载列表中。'}
      </p>
      {error && (
        <p
          role="alert"
          className="break-words rounded-lg border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      {notice && (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-primary/5 px-4 py-3 text-sm"
        >
          <p className="flex min-w-0 items-start gap-2">
            <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-primary" />
            {notice}
          </p>
          <div className="flex items-center gap-2">
            {addedFiles && (
              <Button variant="ghost" size="sm" disabled={busy} onClick={showAdded}>
                查看新增文件
                <ArrowRight />
              </Button>
            )}
            {insertedStatement && (
              <Button asChild variant="ghost" size="sm">
                <Link
                  to={`/authoring/${problemId}/statement?entry=${encodeURIComponent(insertedStatement)}`}
                >
                  回到题面查看
                  <ArrowRight />
                </Link>
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label="关闭附件操作提示"
              onClick={() => setNotice('')}
            >
              <X />
            </Button>
          </div>
        </div>
      )}
      {uploadProgress && (
        <section aria-label="附件上传进度" className="overflow-hidden rounded-xl border bg-card">
          <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              {['uploading', 'saving'].includes(uploadProgress.phase) ? (
                <Loader2 className="size-4 animate-spin text-primary" />
              ) : uploadProgress.phase === 'saved' ? (
                <CheckCircle2 className="size-4 text-primary" />
              ) : (
                <CircleAlert className="size-4 text-destructive" />
              )}
              <span role="status">
                {uploadProgress.phase === 'saving'
                  ? '文件上传完成，正在写入个人草稿…'
                  : uploadProgress.phase === 'saved'
                    ? `${uploadProgress.files.length} 个文件已添加`
                    : uploadProgress.phase === 'failed'
                      ? '添加未完成，请查看错误提示'
                      : `正在上传 ${uploadProgress.files.filter((file) => file.state === 'uploaded').length} / ${uploadProgress.files.length} 个文件`}
              </span>
            </div>
            {!busy && (
              <Button
                variant="ghost"
                size="icon"
                className="size-7"
                aria-label="关闭上传进度"
                onClick={() => setUploadProgress(undefined)}
              >
                <X />
              </Button>
            )}
          </div>
          <ul className="max-h-48 divide-y overflow-auto">
            {uploadProgress.files.map((item, index) => (
              <li key={index} className="flex items-center justify-between gap-3 px-4 py-2 text-xs">
                <span className="min-w-0 truncate" title={item.name}>
                  {item.name}
                </span>
                <span
                  className={`shrink-0 ${item.state === 'failed' ? 'text-destructive' : 'text-muted-foreground'}`}
                >
                  {uploadProgress.phase === 'saved'
                    ? '已添加到草稿'
                    : item.state === 'uploaded'
                      ? '已上传'
                      : item.state === 'uploading'
                        ? '上传中…'
                        : item.state === 'failed'
                          ? '上传失败'
                          : '等待上传'}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
      <div className="grid min-w-0 items-start gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div
          className="min-w-0"
          onDragOver={(e) => {
            e.preventDefault()
            e.dataTransfer.dropEffect = canEdit && !busy ? 'copy' : 'none'
          }}
          onDrop={(e) => {
            e.preventDefault()
            if (canEdit && !busy) void add(Array.from(e.dataTransfer.files))
          }}
        >
          {files.length ? (
            <div
              className="grid max-h-[65dvh] gap-3 overflow-y-auto p-px sm:grid-cols-2 2xl:grid-cols-3"
              aria-label="附件文件列表"
            >
              {files.map((entry) => {
                const kind = assetType(entry.path)
                const Icon =
                  kind === 'image'
                    ? Image
                    : kind === 'document'
                      ? FileText
                      : kind === 'archive'
                        ? FolderArchive
                        : File
                return (
                  <button
                    key={entry.id}
                    disabled={busy}
                    aria-pressed={file?.id === entry.id}
                    onClick={() => selectFile(entry.id)}
                    title={entryLabel(entry)}
                    className={`min-w-0 rounded-xl border p-4 text-left transition-colors ${file?.id === entry.id ? 'border-primary bg-primary/5 ring-1 ring-primary/15' : 'bg-card hover:border-primary/40'}`}
                  >
                    <Icon className="mb-4 size-7 text-muted-foreground" />
                    <span className="block truncate text-sm font-medium">
                      {entry.attributes.label || entry.path.split('/').pop()}
                    </span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {typeLabels[kind]} · {formatFileSize(entry.blob.bytes)}
                    </span>
                  </button>
                )
              })}
            </div>
          ) : (
            <div className="rounded-xl border-2 border-dashed bg-muted/15 px-6 py-16 text-center">
              <Upload className="mx-auto mb-3 size-7 text-muted-foreground" />
              <p className="text-sm font-medium">
                {filtered
                  ? '没有匹配的文件'
                  : canEdit
                    ? '把文件拖到这里，或选择文件上传'
                    : `还没有${scope === 'public' ? '公开附件' : '内部资料'}`}
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                {filtered
                  ? '试试其他名称或类型，也可以清除筛选查看全部文件。'
                  : scope === 'public'
                    ? '图片上传后可以直接插入题面。'
                    : '保存出题过程中的说明、参考资料或辅助文件。'}
              </p>
              {filtered ? (
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-4"
                  disabled={busy}
                  onClick={clearFilters}
                >
                  清除筛选
                </Button>
              ) : (
                canEdit && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-4"
                    disabled={busy}
                    onClick={() => upload.current?.click()}
                  >
                    <Upload />
                    选择文件
                  </Button>
                )
              )}
            </div>
          )}
          {canEdit && (
            <p className="mt-3 text-xs text-muted-foreground">
              支持拖入文件 · 每次最多 100 个 · 单文件不超过 64 MiB。上传后不会自动插入题面。
            </p>
          )}
        </div>
        <aside
          ref={details}
          tabIndex={-1}
          aria-label="附件详情"
          className="min-w-0 scroll-mt-[calc(var(--app-header-height)+1rem)] rounded-xl border bg-card p-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring lg:sticky lg:top-[calc(var(--app-header-height)+1rem)]"
        >
          {file ? (
            <div className="space-y-4">
              <h3 className="break-words text-sm font-semibold">
                {file.attributes.label || file.path.split('/').pop()}
              </h3>
              <p className="text-xs text-muted-foreground">
                {typeLabels[assetType(file.path)]} · {formatFileSize(file.blob.bytes)} ·{' '}
                {assetScope(file) === 'public' ? '公开附件' : '内部资料'}
              </p>
              {!files.some((entry) => entry.id === file.id) && (
                <div className="rounded-lg bg-muted/30 p-3 text-xs">
                  <p className="text-muted-foreground">当前选择不在筛选结果中。</p>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="mt-1"
                    disabled={busy}
                    onClick={clearFilters}
                  >
                    清除筛选并显示文件
                  </Button>
                </div>
              )}
              {preview?.state === 'error' ? (
                <div role="alert" className="rounded-lg border border-destructive/25 p-4 text-sm">
                  <p className="text-destructive">
                    {preview.error || '图片无法预览，请重试或下载查看。'}
                  </p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-3"
                    onClick={() => setPreviewAttempt((value) => value + 1)}
                  >
                    <RefreshCw />
                    重试预览
                  </Button>
                </div>
              ) : preview?.image ? (
                <div className="relative">
                  <img
                    src={preview.image}
                    alt={file.attributes.label || '附件预览'}
                    className="max-h-64 w-full rounded-lg border bg-white object-contain"
                    onLoad={() =>
                      setPreviewResult((current) =>
                        current?.key === previewKey ? { ...current, state: 'ready' } : current,
                      )
                    }
                    onError={() =>
                      setPreviewResult((current) =>
                        current?.key === previewKey
                          ? {
                              ...current,
                              state: 'error',
                              error: '图片无法解码，可重试预览或下载原文件。',
                            }
                          : current,
                      )
                    }
                  />
                  {preview.state === 'loading' && (
                    <div
                      role="status"
                      className="absolute inset-0 flex items-center justify-center gap-2 rounded-lg bg-muted/90 text-xs text-muted-foreground"
                    >
                      <Loader2 className="size-4 animate-spin" />
                      正在载入图片…
                    </div>
                  )}
                </div>
              ) : preview?.state === 'ready' && preview.text !== undefined ? (
                preview.text ? (
                  <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/35 p-3 text-xs leading-6">
                    {preview.text}
                  </pre>
                ) : (
                  <p className="rounded-lg bg-muted/30 p-5 text-center text-xs text-muted-foreground">
                    这是一个空文本文件。
                  </p>
                )
              ) : previewKind !== 'unavailable' && (!preview || preview.state === 'loading') ? (
                <div
                  role="status"
                  className="flex h-36 items-center justify-center gap-2 rounded-lg bg-muted/30 text-xs text-muted-foreground"
                >
                  <Loader2 className="size-4 animate-spin" />
                  正在读取预览…
                </div>
              ) : (
                <div className="flex min-h-36 flex-col items-center justify-center gap-3 rounded-lg bg-muted/30 p-4 text-center">
                  <File className="size-10 text-muted-foreground" />
                  <p className="text-xs text-muted-foreground">
                    此文件格式或大小不支持在线预览，可下载查看原文件。
                  </p>
                </div>
              )}
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={() => void download()}
                  disabled={busy || downloading}
                  loading={downloading}
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
              <div className="space-y-2 border-t pt-4">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-medium text-muted-foreground">材料路径</p>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy || copying}
                    onClick={() => void copyText(file.path, '材料路径')}
                  >
                    <Copy />
                    复制路径
                  </Button>
                </div>
                <code
                  tabIndex={0}
                  className="block select-text break-all rounded-lg bg-muted/30 p-3 text-xs leading-5"
                >
                  {file.path}
                </code>
              </div>
              {assetScope(file) === 'public' && (
                <div className="space-y-3 border-t pt-4">
                  {statements.length > 0 && (
                    <Choice
                      label="对应题面"
                      value={statement?.id ?? ''}
                      onChange={setStatementId}
                      options={statements.map((s) => [s.id, entryLabel(s)])}
                      disabled={busy}
                    />
                  )}
                  {!!reference && (
                    <>
                      <pre
                        tabIndex={0}
                        className="max-h-32 select-text overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/30 p-3 text-xs leading-5"
                      >
                        {reference}
                      </pre>
                      <Button
                        variant="outline"
                        className="w-full"
                        disabled={busy || copying}
                        onClick={() => void copyText(reference, '题面引用')}
                      >
                        <Copy />
                        复制题面引用
                      </Button>
                    </>
                  )}
                  {referenceError && (
                    <p className="text-xs text-muted-foreground">{referenceError}</p>
                  )}
                  {canEdit && (
                    <Button
                      variant="outline"
                      className="w-full"
                      disabled={
                        busy || !statement || !reference || statement.blob.bytes > 1024 * 1024
                      }
                      onClick={() => void insert()}
                    >
                      <Link2 />
                      插入题面末尾
                    </Button>
                  )}
                  {statement && (
                    <Link
                      className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                      to={`/authoring/${problemId}/statement?entry=${encodeURIComponent(statement.id)}`}
                    >
                      打开所选题面
                      <ArrowRight className="size-3" />
                    </Link>
                  )}
                  {statement && statement.blob.bytes > 1024 * 1024 && (
                    <p className="text-xs text-muted-foreground">
                      题面超过 1 MiB，请复制引用后在题面编辑器中手动插入。
                    </p>
                  )}
                  {!statement && (
                    <p className="text-xs text-muted-foreground">
                      添加 Markdown 或 TeX 题面后可直接插入。
                    </p>
                  )}
                </div>
              )}
              {assetScope(file) === 'private' && (
                <p className="border-t pt-4 text-xs leading-5 text-muted-foreground">
                  内部资料不生成选手可访问的题面引用，仅供出题协作使用。
                </p>
              )}
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
