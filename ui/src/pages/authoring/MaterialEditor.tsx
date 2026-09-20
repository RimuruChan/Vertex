import { metadataError } from './editor-validation'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type MutableRefObject,
} from 'react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type { DomainBlobRef, DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { Textarea, Input } from '@/components/ui/input'
import { SaveButton } from '@/components/ui/save-button'
import CodeEditor from '@/components/CodeEditor'
import StatementPreview from './StatementPreview'
import { apiError, formatFileSize, FormValidationError } from '@/lib/format'
import { isDocument, materialNames } from '@/lib/authoring-materials'
import { useConfirm } from '@/components/ui/confirm-dialog'
import {
  Maximize2,
  Minimize2,
  Trash2,
  MoreHorizontal,
  Heading2,
  Bold,
  Italic,
  List,
  Code,
  Sigma,
} from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import MaterialForm from './MaterialForm'

type Replacement = { name: string; blob: DomainBlobRef; text?: string }

export default function MaterialEditor({
  problemId,
  entry,
  copy,
  canEdit,
  onSaved,
  onDirty,
  onSaving,
  beforeLeave,
}: {
  problemId: string
  entry: DomainTreeEntry
  copy: DomainWorkingCopy
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onDirty: (dirty: boolean) => void
  onSaving: (saving: boolean) => void
  beforeLeave?: MutableRefObject<(() => Promise<boolean>) | null>
}) {
  const api = useDomainAPI(),
    confirm = useConfirm(),
    [text, setText] = useState(''),
    [original, setOriginal] = useState(''),
    [path, setPath] = useState(entry.path)
  const [loading, setLoading] = useState(true),
    [saving, setSaving] = useState(false),
    [error, setError] = useState(''),
    [uneditable, setUneditable] = useState(false)
  const [saved, setSaved] = useState(false),
    current = useRef(0),
    ownSave = useRef(''),
    savingLock = useRef(false)
  const [replacement, setReplacement] = useState<Replacement>()
  const [showLocation, setShowLocation] = useState(false)
  const replacementInput = useRef<HTMLInputElement>(null)
  const fullscreenRoot = useRef<HTMLDivElement>(null)
  const [fullscreen, setFullscreen] = useState(false),
    [displayError, setDisplayError] = useState('')
  useEffect(() => {
    const changed = () => setFullscreen(document.fullscreenElement === fullscreenRoot.current)
    document.addEventListener('fullscreenchange', changed)
    return () => document.removeEventListener('fullscreenchange', changed)
  }, [])
  async function toggleFullscreen() {
    setDisplayError('')
    try {
      if (document.fullscreenElement === fullscreenRoot.current) await document.exitFullscreen()
      else await fullscreenRoot.current?.requestFullscreen()
    } catch {
      setDisplayError('当前浏览器无法进入全屏，编辑内容仍保留。')
    }
  }
  const [remote, setRemote] = useState<{
      copy: DomainWorkingCopy
      entry?: DomainTreeEntry
      text?: string
    }>(),
    [comparing, setComparing] = useState(false)
  const binary =
    entry.kind === 'asset' ||
    entry.kind === 'resource' ||
    entry.attributes.format === 'pdf' ||
    entry.blob.bytes > 1 << 20 ||
    uneditable
  const formError = entry.kind === 'metadata' && !loading ? metadataError(text) : ''
  const dirty = text !== original || path !== entry.path || Boolean(replacement)
  const [editorMode, setEditorMode] = useState<'edit' | 'split' | 'preview'>('split')
  const [wide, setWide] = useState(() => window.matchMedia('(min-width:1280px)').matches)
  useEffect(() => {
    const media = window.matchMedia('(min-width:1280px)')
    const update = () => setWide(media.matches)
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])
  const displayMode = editorMode === 'split' && !wide ? 'edit' : editorMode
  const editorCommands = useRef<{
    insert: (before: string, after?: string, placeholder?: string) => void
  } | null>(null)
  const markdown = entry.kind === 'statement' && entry.attributes.format === 'markdown'
  useEffect(() => {
    if (ownSave.current === `${entry.id}:${entry.blob.sha256}`) {
      setLoading(false)
      return
    }
    const sequence = ++current.current
    setLoading(true)
    setError('')
    setPath(entry.path)
    setSaved(false)
    if (binary) {
      setLoading(false)
      return
    }
    api
      .getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256)
      .then(async (blob) => {
        try {
          const text = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
          if (text.includes('\0')) throw new Error('binary content')
          return text
        } catch {
          if (sequence === current.current) {
            setUneditable(true)
            setLoading(false)
          }
          return null
        }
      })
      .then((value) => {
        if (sequence !== current.current || value === null) return
        let content = value
        if (isDocument(entry.kind)) {
          try {
            content = JSON.stringify(JSON.parse(value), null, 2)
          } catch {}
        }
        setText(content)
        setOriginal(content)
        setLoading(false)
      })
      .catch((error) => {
        if (sequence === current.current) {
          setError(apiError(error, '读取材料失败'))
          setLoading(false)
        }
      })
    return () => {
      current.current++
    }
  }, [entry.id, entry.blob.sha256, problemId, api, binary])
  useEffect(() => {
    onDirty(dirty)
    return () => onDirty(false)
  }, [dirty, onDirty])
  useEffect(() => {
    onSaving(saving)
    return () => onSaving(false)
  }, [saving, onSaving])
  const save = useCallback(
    async (against?: DomainWorkingCopy, file = replacement) => {
      if (savingLock.current || !canEdit || loading || formError) return
      const sequence = current.current
      savingLock.current = true
      setSaving(true)
      setError('')
      try {
        const next = await api.putApiAuthoringProblemsIdWorkingCopyEntriesEntryId(
          problemId,
          entry.id,
          {
            etag: against?.etag ?? copy.etag,
            entry: { ...entry, path, ...(file ? { blob: file.blob } : {}) },
            ...(binary || file ? {} : { text }),
          },
        )
        if (sequence !== current.current) return
        ownSave.current = `${entry.id}:${next.tree.entries.find((e) => e.id === entry.id)?.blob.sha256}`
        if (file) {
          setText(file.text ?? '')
          setOriginal(file.text ?? '')
          setUneditable(file.text === undefined)
          setReplacement(undefined)
        } else setOriginal(text)
        setSaved(true)
        setRemote(undefined)
        onSaved(next)
        return true
      } catch (error) {
        if (sequence === current.current) setError(apiError(error, '保存失败，本地编辑已保留'))
        return false
      } finally {
        savingLock.current = false
        if (sequence === current.current) setSaving(false)
      }
    },
    [
      api,
      problemId,
      entry,
      path,
      text,
      copy.etag,
      onSaved,
      canEdit,
      binary,
      loading,
      replacement,
      formError,
    ],
  )
  useLayoutEffect(() => {
    if (!beforeLeave) return
    beforeLeave.current = async () => !dirty || Boolean(await save())
    return () => {
      beforeLeave.current = null
    }
  }, [beforeLeave, dirty, save])
  async function replaceFile(file: File) {
    if (savingLock.current || !canEdit || loading) return
    const sequence = current.current
    savingLock.current = true
    setSaving(true)
    setError('')
    setSaved(false)
    try {
      if (file.size > 64 * 1024 * 1024) throw new FormValidationError('替换文件不能超过 64 MiB。')
      let value: string | undefined
      if (
        file.size <= 1 << 20 &&
        !['asset', 'resource'].includes(entry.kind) &&
        entry.attributes.format !== 'pdf'
      ) {
        try {
          value = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer())
          if (value.includes('\0')) value = undefined
          else if (isDocument(entry.kind)) value = JSON.stringify(JSON.parse(value), null, 2)
        } catch {
          value = undefined
        }
      }
      const blob = await api.postApiAuthoringProblemsIdBlobs(problemId, { file })
      if (sequence !== current.current) return
      const pending = { name: file.name, blob, text: value }
      setReplacement(pending)
      savingLock.current = false
      await save(undefined, pending)
    } catch (error) {
      if (sequence === current.current) setError(apiError(error, '替换失败，原文件仍保留'))
    } finally {
      savingLock.current = false
      if (sequence === current.current) setSaving(false)
      if (replacementInput.current) replacementInput.current.value = ''
    }
  }
  useEffect(() => {
    if (!dirty || saving || loading || binary || !canEdit || error || remote || formError) return
    const timer = window.setTimeout(() => void save(), 900)
    return () => window.clearTimeout(timer)
  }, [dirty, saving, loading, binary, canEdit, error, remote, save, formError])
  async function compareRemote() {
    setComparing(true)
    try {
      const latest = await api.getApiAuthoringProblemsIdWorkingCopy(problemId)
      const found = latest.tree.entries.find((item) => item.id === entry.id)
      let remoteText: string | undefined
      if (found && found.blob.bytes <= 1 << 20) {
        const data = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, found.blob.sha256)
        try {
          remoteText = new TextDecoder('utf-8', { fatal: true }).decode(await data.arrayBuffer())
          if (remoteText.includes('\0')) remoteText = undefined
        } catch {
          remoteText = undefined
        }
      }
      setRemote({ copy: latest, entry: found, text: remoteText })
    } catch (error) {
      setError(apiError(error, '无法读取服务器副本，本地输入仍保留'))
    } finally {
      setComparing(false)
    }
  }
  function useRemote() {
    if (!remote) return
    setReplacement(undefined)
    setUneditable(
      Boolean(remote.entry && remote.text === undefined && remote.entry.blob.bytes <= 1 << 20),
    )
    if (remote.entry && remote.text === undefined) {
      setText('')
      setOriginal('')
      setPath(remote.entry.path)
      ownSave.current = ''
    }
    if (remote.entry && remote.text !== undefined) {
      let value = remote.text
      if (isDocument(entry.kind)) {
        try {
          value = JSON.stringify(JSON.parse(value), null, 2)
        } catch {}
      }
      setText(value)
      setOriginal(value)
      setPath(remote.entry.path)
      ownSave.current = `${entry.id}:${remote.entry.blob.sha256}`
    }
    setError('')
    setSaved(false)
    onSaved(remote.copy)
    setRemote(undefined)
  }
  async function download() {
    try {
      const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256)
      const url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = entry.path.split('/').pop() || 'file'
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      setError(apiError(error, '下载失败'))
    }
  }
  async function remove() {
    if (
      !(await confirm({
        title: '从工作副本移除此材料？',
        description: '已有提交与发布版本保留。引用此材料的程序或测试可能需要调整。',
        confirmLabel: '移除材料',
        destructive: true,
      }))
    )
      return
    setSaving(true)
    setError('')
    try {
      onSaved(
        await api.deleteApiAuthoringProblemsIdWorkingCopyEntriesEntryId(problemId, entry.id, {
          etag: copy.etag,
        }),
      )
    } catch (error) {
      setError(apiError(error, '移除失败，工作副本未改变'))
    } finally {
      setSaving(false)
    }
  }
  return (
    <div
      ref={fullscreenRoot}
      className={`min-w-0 space-y-5 ${fullscreen ? 'overflow-auto bg-background p-5 sm:p-8' : ''}`}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        {entry.kind === 'statement' ? (
          <h2 className="text-sm font-medium text-muted-foreground">
            {markdown ? 'Markdown' : 'TeX'} 编辑
          </h2>
        ) : (
          <div className="min-w-0">
            <h2 className="text-xl font-semibold tracking-tight">
              {materialNames[entry.kind] || '材料'}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {entry.kind === 'metadata'
                ? '先设置题目与判定规则，再准备内容和数据。'
                : entry.kind === 'statement'
                  ? '专注描述、输入与输出。编辑内容会自动保存。'
                  : entry.path.split('/').pop()}
            </p>
          </div>
        )}
        <div className="flex items-center gap-2">
          {['statement', 'source'].includes(entry.kind) && !uneditable && (
            <Button
              variant="outline"
              size="icon"
              disabled={loading}
              aria-label={fullscreen ? '退出全屏编辑' : '全屏编辑'}
              onClick={() => void toggleFullscreen()}
            >
              {fullscreen ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
            </Button>
          )}
          {canEdit && (
            <SaveButton
              loading={saving}
              saved={saved && !dirty}
              disabled={
                loading || comparing || Boolean(remote) || !!formError || (!dirty && !saved)
              }
              onClick={() => void save()}
            >
              保存副本
            </SaveButton>
          )}
          {canEdit && entry.kind === 'statement' && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="icon" aria-label="题面操作">
                  <MoreHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setShowLocation((value) => !value)}>
                  文件位置与重命名
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={saving || dirty || loading}
                  onSelect={() => void remove()}
                >
                  移除这份题面
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
          {canEdit && entry.id !== 'problem' && entry.kind !== 'statement' && (
            <Button
              variant="outline"
              size="icon"
              aria-label="移除当前材料"
              disabled={saving || dirty || loading}
              onClick={() => void remove()}
            >
              <Trash2 className="size-4" />
            </Button>
          )}
        </div>
      </div>
      {displayError && (
        <p role="alert" className="text-sm text-muted-foreground">
          {displayError}
        </p>
      )}
      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-3 space-y-2">
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
          {canEdit && (
            <Button
              variant="outline"
              size="sm"
              disabled={saving || comparing}
              loading={comparing}
              onClick={() => void compareRemote()}
            >
              核对服务器副本
            </Button>
          )}
        </div>
      )}
      {remote && (
        <section className="space-y-3 rounded-xl border p-4" aria-label="保存冲突核对">
          <h3 className="text-sm font-medium">保留本地输入，核对另一页面的更改</h3>
          <p className="text-xs text-muted-foreground">
            服务器路径：{remote.entry?.path ?? '此材料已删除'} · 本地路径：{path}
          </p>
          <div className="grid min-w-0 gap-3 lg:grid-cols-2">
            {[
              [
                '服务器最新内容',
                remote.text ??
                  (remote.entry
                    ? '文件过大或是二进制，可采用服务器版本后下载查看。'
                    : '材料已删除'),
              ],
              [
                '本地未保存内容',
                replacement
                  ? (replacement.text ??
                    `${replacement.name}\n${formatFileSize(replacement.blob.bytes)} · 待保存的替换文件`)
                  : text,
              ],
            ].map(([title, body]) => (
              <div key={title} className="min-w-0 rounded-lg border">
                <h4 className="border-b px-3 py-2 text-xs font-medium">{title}</h4>
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words p-3 text-xs leading-5">
                  {body}
                </pre>
              </div>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={saving} onClick={useRemote}>
              采用服务器版本
            </Button>
            <Button disabled={saving} onClick={() => void save(remote.copy)}>
              保留本地并保存此材料
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            只替换当前材料，服务器副本中的其他材料保持不变。如果服务器又有更新，会再次核对。
          </p>
        </section>
      )}
      {replacement && (
        <p role="status" className="break-words rounded-lg border p-3 text-sm">
          待保存的替换：{replacement.name} · {formatFileSize(replacement.blob.bytes)}
          。核对后保存，原文件路径和引用保持不变。
        </p>
      )}
      {loading ? (
        <p className="py-12 text-sm text-muted-foreground">正在读取材料…</p>
      ) : binary ? (
        <div className="surface-panel space-y-3 p-6">
          {canEdit && (
            <div className="space-y-2">
              <label htmlFor="binary-material-path" className="text-sm font-medium">
                文件路径
              </label>
              <Input
                id="binary-material-path"
                value={path}
                onChange={(event) => {
                  setPath(event.target.value)
                  setSaved(false)
                }}
              />
              <p className="text-xs text-muted-foreground">
                重命名保留原始文件字节，点击保存副本完成。
              </p>
            </div>
          )}
          <p className="text-sm text-muted-foreground">
            {formatFileSize(entry.blob.bytes)} · 此文件保留原始格式。
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => void download()}>
              下载文件
            </Button>
            {canEdit && (
              <>
                <input
                  ref={replacementInput}
                  className="hidden"
                  type="file"
                  aria-label="选择替换文件"
                  accept={entry.attributes.format === 'pdf' ? '.pdf,application/pdf' : undefined}
                  onChange={(event) => {
                    const file = event.target.files?.[0]
                    if (file) void replaceFile(file)
                  }}
                />
                <Button
                  variant="outline"
                  disabled={saving || comparing}
                  onClick={() => replacementInput.current?.click()}
                >
                  替换文件
                </Button>
              </>
            )}
          </div>
          {canEdit && (
            <p className="text-xs text-muted-foreground">
              替换会保存到工作副本，保留测试点引用；旧提交和发布版本不变。
            </p>
          )}
        </div>
      ) : (
        <>
          {entry.id !== 'problem' && canEdit && (entry.kind !== 'statement' || showLocation) && (
            <details
              open={entry.kind === 'statement' ? true : undefined}
              className="max-w-lg text-sm"
            >
              <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                重命名文件
              </summary>
              <div className="mt-3 space-y-1.5">
                <label htmlFor="material-path" className="text-sm font-medium">
                  文件路径
                </label>
                <Input
                  id="material-path"
                  value={path}
                  disabled={!canEdit}
                  onChange={(e) => {
                    setPath(e.target.value)
                    setSaved(false)
                  }}
                />
              </div>
            </details>
          )}
          {isDocument(entry.kind) ? (
            <MaterialForm
              problemId={problemId}
              revision={!canEdit ? copy.baseRevision : undefined}
              kind={entry.kind}
              text={text}
              entries={copy.tree.entries.filter((e) => e.id !== entry.id)}
              disabled={!canEdit}
              onChange={(value) => {
                setText(value)
                setError('')
                setSaved(false)
              }}
            />
          ) : entry.kind === 'source' ? (
            <div
              className={`${fullscreen ? 'h-[calc(100dvh-14rem)]' : 'h-[min(65dvh,720px)]'} overflow-hidden rounded-xl border`}
            >
              <CodeEditor
                value={text}
                language={entry.attributes.language || 'cpp'}
                ariaLabel="程序源代码"
                readOnly={!canEdit}
                documentKey={entry.id}
                onChange={(value) => {
                  setText(value)
                  setError('')
                  setSaved(false)
                }}
              />
            </div>
          ) : entry.kind === 'statement' ? (
            <div className="overflow-hidden rounded-xl border bg-card">
              <div className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b bg-muted/25 px-3 py-2">
                <div className="flex items-center gap-1">
                  {markdown ? (
                    <>
                      {[
                        ['标题', '## ', '', '标题'],
                        ['加粗', '**', '**', '文字'],
                        ['斜体', '*', '*', '文字'],
                        ['列表', '- ', '', '列表项'],
                        ['代码', '`', '`', '代码'],
                        ['公式', '$', '$', 'a+b'],
                      ].map(([label, before, after, placeholder], index) => {
                        const Icon = [Heading2, Bold, Italic, List, Code, Sigma][index]
                        return (
                          <Button
                            key={label}
                            size="icon"
                            className="size-8"
                            aria-label={label}
                            title={label}
                            variant="ghost"
                            disabled={!canEdit || displayMode === 'preview'}
                            onClick={() =>
                              editorCommands.current?.insert(before, after, placeholder)
                            }
                          >
                            <Icon className="size-4" />
                          </Button>
                        )
                      })}
                    </>
                  ) : (
                    <span className="px-2 text-xs text-muted-foreground">
                      TeX 源文件 · 保存后在交付页编译预览
                    </span>
                  )}
                </div>
                {markdown && (
                  <div className="flex rounded-lg border bg-background p-0.5">
                    {(['edit', 'split', 'preview'] as const).map((mode) => (
                      <button
                        key={mode}
                        type="button"
                        onClick={() => setEditorMode(mode)}
                        aria-pressed={displayMode === mode}
                        className={`rounded-md px-3 py-1.5 text-xs transition-colors ${mode === 'split' ? 'hidden xl:block' : ''} ${displayMode === mode ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:text-foreground'}`}
                      >
                        {{ edit: '编辑', split: '分屏', preview: '预览' }[mode]}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <div
                className={`grid min-h-[480px] ${fullscreen ? 'h-[calc(100dvh-14rem)]' : 'h-[min(68dvh,760px)]'} ${markdown && displayMode === 'split' ? 'xl:grid-cols-2' : ''}`}
              >
                <div
                  className={`min-h-0 min-w-0 ${markdown && displayMode === 'preview' ? 'hidden' : ''}`}
                >
                  <CodeEditor
                    commands={editorCommands}
                    value={text}
                    language="text"
                    ariaLabel="题面内容"
                    documentKey={entry.id}
                    readOnly={!canEdit}
                    className="rounded-none border-0"
                    onChange={(next) => {
                      setText(next)
                      setSaved(false)
                      setError('')
                    }}
                  />
                </div>
                {markdown && displayMode !== 'edit' && (
                  <div
                    className={`min-w-0 overflow-auto bg-background p-6 sm:p-8 ${displayMode === 'split' ? 'hidden border-l xl:block' : ''}`}
                  >
                    <StatementPreview
                      problemId={problemId}
                      entry={entry}
                      entries={copy.tree.entries}
                      content={text}
                    />
                  </div>
                )}
              </div>
              <div className="flex justify-between border-t px-4 py-2 text-xs text-muted-foreground">
                <span>
                  {markdown ? 'Markdown' : 'TeX'} · {text.length.toLocaleString()} 字
                </span>
                <span>{saving ? '正在保存…' : dirty ? '等待保存' : '已保存'}</span>
              </div>
            </div>
          ) : (
            <Textarea
              aria-label="数据内容"
              className="min-h-80 resize-none font-mono"
              value={text}
              disabled={!canEdit}
              onChange={(e) => {
                setText(e.target.value)
                setSaved(false)
                setError('')
              }}
            />
          )}
          {!['statement', 'metadata'].includes(entry.kind) && (
            <p className="text-xs text-muted-foreground">
              {!canEdit
                ? '正在审阅共享提交，内容为只读。'
                : dirty
                  ? error
                    ? '自动保存已暂停，本地输入仍保留。'
                    : '编辑停顿后自动保存到工作副本。'
                  : '已保存在工作副本中。'}{' '}
            </p>
          )}
        </>
      )}
    </div>
  )
}
