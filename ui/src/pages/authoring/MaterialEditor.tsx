import { metadataError } from './editor-validation'
import { availableLocation } from './material-locations'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
  type ReactNode,
} from 'react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import { useAuth } from '@/auth/AuthContext'
import type { DomainBlobRef, DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/input'
import { SaveButton } from '@/components/ui/save-button'
import CodeEditor from '@/components/CodeEditor'
import StatementComposer from './StatementComposer'
import { apiError, formatFileSize, FormValidationError } from '@/lib/format'
import { isDocument, materialNames, entryLabel } from '@/lib/authoring-materials'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Check, Focus, Loader2, Maximize2, Minimize2, Trash2, MoreHorizontal } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import MaterialForm from './MaterialForm'
import { useEditorFocus } from './useEditorFocus'
import ReviewTextDiff from './ReviewTextDiff'
import {
  afterStatementSave,
  discardStatementDraft,
  readStatementDraft,
  statementDraftKey,
  statementRecoveryState,
  writeStatementDraft,
  type StatementDraft,
} from './statement-draft-recovery'

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
  statementNavigation,
}: {
  problemId: string
  entry: DomainTreeEntry
  copy: DomainWorkingCopy
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onDirty: (dirty: boolean) => void
  onSaving: (saving: boolean) => void
  statementNavigation?: ReactNode
  beforeLeave?: MutableRefObject<(() => Promise<boolean>) | null>
}) {
  const api = useDomainAPI(),
    { user } = useAuth(),
    { slug } = useDomain(),
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
    savingLock = useRef(false),
    inputVersion = useRef(0)
  const [replacement, setReplacement] = useState<Replacement>()
  const [recovery, setRecovery] = useState<StatementDraft>(),
    [manualRecovery, setManualRecovery] = useState(false),
    [sessionStored, setSessionStored] = useState(true),
    [loadedStatementHash, setLoadedStatementHash] = useState<string>()
  const localDraft = useRef<StatementDraft | undefined>(undefined),
    draftBaseline = useRef({ hash: entry.blob.sha256, text: '' })
  const sessionStore = useMemo(() => {
    try {
      return window.sessionStorage
    } catch {
      return undefined
    }
  }, [])
  const replacementInput = useRef<HTMLInputElement>(null)
  const fullscreenRoot = useRef<HTMLDivElement>(null)
  const [focused, setFocused] = useState(false)
  const exitFocus = useCallback(() => setFocused(false), [])
  useEditorFocus(fullscreenRoot, focused, exitFocus)
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
  const markdown = entry.kind === 'statement' && entry.attributes.format === 'markdown'
  const recoveryKey =
    entry.kind === 'statement' &&
    ['markdown', 'tex'].includes(entry.attributes.format) &&
    !binary &&
    user?.id
      ? statementDraftKey({
          userId: String(user.id),
          domainSlug: slug,
          problemId,
          entryId: entry.id,
        })
      : undefined
  const statementReady = !recoveryKey || loadedStatementHash === entry.blob.sha256
  const inspectRecovery = useCallback(
    (serverText: string, serverHash: string) => {
      setLoadedStatementHash(serverHash)
      draftBaseline.current = { text: serverText, hash: serverHash }
      const draft = recoveryKey ? readStatementDraft(sessionStore, recoveryKey) : undefined
      localDraft.current = draft
      setManualRecovery(false)
      if (
        draft &&
        canEdit &&
        statementRecoveryState(draft, serverText, serverHash) === 'already-saved'
      ) {
        discardStatementDraft(sessionStore, recoveryKey!, draft.inputVersion)
        localDraft.current = undefined
        setRecovery(undefined)
      } else setRecovery(draft)
    },
    [canEdit, recoveryKey, sessionStore],
  )
  const readOnlyStatus = copy.mergeId
    ? '请先解决版本冲突，当前内容为只读'
    : copy.baseRevision
      ? `正在审阅 r${copy.baseRevision}，内容为只读`
      : '当前内容为只读'
  function restoreRecovery() {
    if (!canEdit || !recovery || loading || saving || comparing) return
    inputVersion.current++
    localDraft.current = recovery
    setText(recovery.text)
    setRecovery(undefined)
    setManualRecovery(true)
    setSessionStored(true)
    setSaved(false)
    setError('')
    setDisplayError('')
  }
  function discardRecovery() {
    if (!recovery || !recoveryKey) return
    discardStatementDraft(sessionStore, recoveryKey, recovery.inputVersion)
    localDraft.current = undefined
    setRecovery(undefined)
    setManualRecovery(false)
    setDisplayError('')
  }
  function changeText(value: string) {
    inputVersion.current++
    setText(value)
    setError('')
    setSaved(false)
    if (recoveryKey && canEdit && statementReady && !loading && !recovery) {
      if (value === draftBaseline.current.text && !savingLock.current) {
        if (localDraft.current)
          discardStatementDraft(sessionStore, recoveryKey, localDraft.current.inputVersion)
        localDraft.current = undefined
        setManualRecovery(false)
      } else {
        const draft: StatementDraft = {
          schemaVersion: 1,
          inputVersion: crypto.randomUUID(),
          text: value,
          baselineHash: localDraft.current?.baselineHash ?? draftBaseline.current.hash,
          updatedAt: Date.now(),
        }
        localDraft.current = draft
        setSessionStored(writeStatementDraft(sessionStore, recoveryKey, draft))
      }
    }
  }
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
        inspectRecovery(content, entry.blob.sha256)
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
  }, [entry.id, entry.blob.sha256, problemId, api, binary, inspectRecovery])
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
      if (
        savingLock.current ||
        !canEdit ||
        loading ||
        comparing ||
        !statementReady ||
        recovery ||
        (remote && !against) ||
        formError
      )
        return
      const sequence = current.current
      const savedDraft = localDraft.current
      const savedInputVersion = inputVersion.current
      savingLock.current = true
      setSaving(true)
      setError('')
      try {
        const next = await api.putApiAuthoringProblemsIdWorkingCopyEntriesEntryId(
          problemId,
          entry.id,
          {
            etag: against?.etag ?? copy.etag,
            entry: {
              ...entry,
              path,
              ...(isDocument(entry.kind) &&
              !binary &&
              !file &&
              typeof JSON.parse(text).name === 'string'
                ? { attributes: { ...entry.attributes, label: JSON.parse(text).name } }
                : {}),
              ...(file ? { blob: file.blob } : {}),
            },
            ...(binary || file ? {} : { text }),
          },
        )
        if (sequence !== current.current) return
        ownSave.current = `${entry.id}:${next.tree.entries.find((e) => e.id === entry.id)?.blob.sha256}`
        if (recoveryKey && !file) {
          const hash =
            next.tree.entries.find((item) => item.id === entry.id)?.blob.sha256 ?? entry.blob.sha256
          const remaining = afterStatementSave(localDraft.current, savedDraft, hash)
          draftBaseline.current = { text, hash }
          setLoadedStatementHash(hash)
          localDraft.current = remaining
          if (remaining) setSessionStored(writeStatementDraft(sessionStore, recoveryKey, remaining))
          else if (savedDraft) {
            discardStatementDraft(sessionStore, recoveryKey, savedDraft.inputVersion)
            setSessionStored(true)
          }
          if (savedInputVersion === inputVersion.current) setManualRecovery(false)
        }
        if (file) {
          setText(file.text ?? '')
          setOriginal(file.text ?? '')
          setUneditable(file.text === undefined)
          setReplacement(undefined)
        } else setOriginal(text)
        setSaved(true)
        setRemote(undefined)
        onSaved(next)
        return next
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
      comparing,
      remote,
      recovery,
      recoveryKey,
      sessionStore,
      statementReady,
    ],
  )
  useLayoutEffect(() => {
    if (!beforeLeave) return
    beforeLeave.current = async () => {
      if (recovery) return true // The untouched session draft remains available on return.
      if (manualRecovery) {
        if (!sessionStored) {
          setDisplayError('浏览器未能暂存恢复后的输入，请先明确保存，再切换材料。')
          return false
        }
        return confirm({
          title: '保留本地草稿并切换？',
          description:
            '恢复的内容尚未写入服务器，会保留在当前标签页会话中。切回后可继续恢复；关闭标签页会结束这份暂存。',
          confirmLabel: '保留草稿并切换',
        })
      }
      if (!dirty) return true
      const version = inputVersion.current
      const result = await save()
      // A save only covers the input it started with. Keep the editor open if
      // more input arrived while the request was pending.
      return Boolean(result) && version === inputVersion.current
    }
    return () => {
      beforeLeave.current = null
    }
  }, [beforeLeave, dirty, save, recovery, manualRecovery, sessionStored, confirm])
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
    if (
      !dirty ||
      saving ||
      loading ||
      binary ||
      !canEdit ||
      error ||
      remote ||
      formError ||
      !statementReady ||
      recovery ||
      manualRecovery
    )
      return
    const timer = window.setTimeout(() => void save(), 900)
    return () => window.clearTimeout(timer)
  }, [
    dirty,
    saving,
    loading,
    binary,
    canEdit,
    error,
    remote,
    save,
    formError,
    recovery,
    manualRecovery,
    statementReady,
  ])
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
    if (recoveryKey && localDraft.current)
      discardStatementDraft(sessionStore, recoveryKey, localDraft.current.inputVersion)
    localDraft.current = undefined
    setRecovery(undefined)
    setManualRecovery(false)
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
      inspectRecovery(value, remote.entry.blob.sha256)
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
      className={
        focused
          ? 'fixed inset-0 z-[45] flex h-dvh min-h-0 min-w-0 flex-col gap-3 overflow-auto bg-background p-2 sm:p-4'
          : `min-w-0 space-y-5 ${fullscreen ? 'overflow-auto bg-background p-5 sm:p-8' : ''}`
      }
      onKeyDownCapture={(event) => {
        if (
          entry.kind === 'statement' &&
          !binary &&
          !loading &&
          event.key === 'Enter' &&
          (event.metaKey || event.ctrlKey) &&
          event.shiftKey &&
          !event.altKey &&
          !event.nativeEvent.isComposing &&
          fullscreenRoot.current?.contains(event.target as Node)
        ) {
          event.preventDefault()
          event.stopPropagation()
          setFocused((value) => !value)
          return
        }
        if (
          event.key.toLowerCase() !== 's' ||
          !(event.metaKey || event.ctrlKey) ||
          event.altKey ||
          event.shiftKey ||
          event.nativeEvent.isComposing ||
          !fullscreenRoot.current?.contains(event.target as Node)
        )
          return
        event.preventDefault()
        event.stopPropagation()
        if (dirty && !saving) void save()
      }}
    >
      {(entry.kind !== 'statement' || binary) && (
        <div className="flex flex-wrap items-start justify-between gap-3">
          {entry.kind === 'statement' ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              {statementNavigation}
              <span>
                {entry.attributes.format === 'pdf'
                  ? 'PDF 题面'
                  : markdown
                    ? 'Markdown 题面'
                    : 'TeX 题面'}
              </span>
            </div>
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
                    : entryLabel(entry)}
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
                <DropdownMenuContent
                  align="end"
                  portalContainer={fullscreen ? fullscreenRoot.current : undefined}
                >
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
      )}
      {displayError && (
        <p role="alert" className="text-sm text-muted-foreground">
          {displayError}
        </p>
      )}
      {!loading && statementReady && recovery && (
        <section
          className="space-y-3 rounded-xl border border-primary/25 bg-primary/5 p-4"
          aria-label="本地题面草稿恢复"
        >
          <div>
            <h3 className="text-sm font-semibold">发现本标签页未保存的题面</h3>
            <p className="mt-1 text-xs leading-6 text-muted-foreground">
              暂存于 {new Date(recovery.updatedAt).toLocaleString()}
              。当前编辑器显示服务器内容，恢复不会立即写入服务器。
            </p>
          </div>
          {recovery.baselineHash !== entry.blob.sha256 && (
            <p className="text-sm text-amber-700 dark:text-amber-400">
              服务器题面已变化。请先核对下方差异；恢复只填入编辑器，明确保存时仍会校验当前服务器副本。
            </p>
          )}
          {!canEdit && (
            <p className="text-sm text-muted-foreground">
              当前为只读版本，不能恢复到编辑器；这份本地草稿会继续保留。
            </p>
          )}
          <details
            open={recovery.baselineHash !== entry.blob.sha256}
            className="rounded-lg border bg-card p-3"
          >
            <summary className="cursor-pointer text-sm">核对服务器内容与本地草稿</summary>
            <p className="my-3 text-xs text-muted-foreground">
              修改前：当前服务器题面；修改后：本地暂存题面。
            </p>
            <ReviewTextDiff before={original} after={recovery.text} />
          </details>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" disabled={!canEdit || saving || comparing} onClick={restoreRecovery}>
              恢复到编辑器
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={saving || comparing}
              onClick={discardRecovery}
            >
              丢弃本地草稿
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            草稿按当前账号、域和题目隔离，只保存在本标签页会话中。
          </p>
        </section>
      )}
      {!loading && manualRecovery && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/25 bg-primary/5 p-3">
          <div>
            <p className="text-sm font-medium">已恢复到编辑器，等待手动保存</p>
            <p className="mt-1 text-xs text-muted-foreground">
              自动保存已暂停；确认内容后再写入当前服务器副本。
            </p>
          </div>
          <Button
            size="sm"
            disabled={!canEdit || saving || comparing || Boolean(remote) || !dirty}
            onClick={() => void save()}
          >
            保存恢复内容
          </Button>
        </div>
      )}
      {recoveryKey && dirty && !sessionStored && (
        <p role="status" className="text-xs text-muted-foreground">
          浏览器未能暂存本地输入。服务器保存仍可使用，请保持页面开启直到保存成功。
        </p>
      )}
      {error && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-3 space-y-2">
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
          {canEdit && (
            <div className="flex flex-wrap gap-2">
              {dirty && !remote && (
                <Button
                  size="sm"
                  disabled={loading || saving || comparing || Boolean(formError)}
                  onClick={() => void save()}
                >
                  重试保存
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                disabled={saving || comparing}
                loading={comparing}
                onClick={() => void compareRemote()}
              >
                核对服务器副本
              </Button>
            </div>
          )}
        </div>
      )}
      {remote && (
        <section className="space-y-3 rounded-xl border p-4" aria-label="保存冲突核对">
          <h3 className="text-sm font-medium">保留本地输入，核对另一页面的更改</h3>
          <p className="text-xs text-muted-foreground">
            {remote.entry
              ? '另一页面已更新这份材料，请选择要保留的内容。'
              : '这份材料已在另一页面删除。'}
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
          。核对后保存，原有引用保持不变。
        </p>
      )}
      {loading ? (
        <p className="py-12 text-sm text-muted-foreground">正在读取材料…</p>
      ) : binary ? (
        <div className="surface-panel space-y-3 p-6">
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
          {isDocument(entry.kind) ? (
            <MaterialForm
              problemId={problemId}
              revision={!canEdit ? copy.baseRevision : undefined}
              kind={entry.kind}
              text={text}
              entries={copy.tree.entries.filter((e) => e.id !== entry.id)}
              disabled={!canEdit}
              onChange={changeText}
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
                onChange={changeText}
              />
            </div>
          ) : entry.kind === 'statement' ? (
            <StatementComposer
              etag={copy.etag}
              revision={!canEdit ? copy.baseRevision : undefined}
              navigation={statementNavigation}
              actions={
                <>
                  {canEdit ? (
                    <Button
                      size="sm"
                      variant={remote ? 'ghost' : error ? 'default' : dirty ? 'outline' : 'ghost'}
                      className="w-28 min-w-28 shrink-0"
                      disabled={
                        loading ||
                        saving ||
                        comparing ||
                        !statementReady ||
                        Boolean(recovery) ||
                        Boolean(remote) ||
                        Boolean(formError) ||
                        !dirty
                      }
                      title="保存到个人草稿 · Ctrl / ⌘ S"
                      onClick={() => void save()}
                    >
                      {saving ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : !dirty && !error && !remote ? (
                        <Check className="size-3.5" />
                      ) : null}
                      <span aria-live="polite" aria-atomic="true">
                        {recovery
                          ? '等待恢复'
                          : remote
                            ? '等待核对'
                            : error
                              ? '重试保存'
                              : saving
                                ? '保存中'
                                : dirty
                                  ? '立即保存'
                                  : '已保存'}
                      </span>
                    </Button>
                  ) : (
                    <span className="text-xs text-muted-foreground">只读</span>
                  )}
                  <Button
                    variant={focused ? 'secondary' : 'ghost'}
                    size="sm"
                    aria-label={focused ? '退出专注编辑' : '专注编辑'}
                    aria-pressed={focused}
                    title={focused ? '退出专注编辑 · Esc' : '专注编辑 · Ctrl / ⌘ Shift Enter'}
                    onClick={() => setFocused((value) => !value)}
                  >
                    {focused ? <Minimize2 /> : <Focus />}
                    <span className="hidden sm:inline">{focused ? '退出专注' : '专注'}</span>
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" aria-label="题面操作">
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent
                      align="end"
                      portalContainer={fullscreen ? fullscreenRoot.current : undefined}
                    >
                      <DropdownMenuItem onSelect={() => void toggleFullscreen()}>
                        {fullscreen ? <Minimize2 /> : <Maximize2 />}
                        {fullscreen ? '退出浏览器全屏' : '浏览器全屏'}
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled={dirty || saving} onSelect={() => void download()}>
                        下载题面
                      </DropdownMenuItem>
                      {canEdit && (
                        <DropdownMenuItem
                          disabled={saving || dirty || Boolean(recovery)}
                          onSelect={() => void remove()}
                        >
                          移除这份题面
                        </DropdownMenuItem>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </>
              }
              problemId={problemId}
              entry={entry}
              entries={copy.tree.entries}
              value={text}
              readOnly={!canEdit || !statementReady || Boolean(remote) || Boolean(recovery)}
              fullscreen={fullscreen}
              focused={focused}
              status={
                !canEdit
                  ? readOnlyStatus
                  : !statementReady
                    ? '等待成功读取服务器题面，本地草稿仍保留'
                    : recovery
                      ? '先选择恢复或丢弃本地草稿；服务器内容尚未改变'
                      : manualRecovery
                        ? '恢复内容等待手动保存 · Ctrl / ⌘ S'
                        : remote
                          ? '请先核对服务器副本，本地输入仍保留'
                          : error
                            ? '保存失败，输入已保留'
                            : saving
                              ? '正在保存…'
                              : dirty
                                ? '等待保存 · Ctrl / ⌘ S 立即保存'
                                : '已保存到个人草稿'
              }
              onChange={changeText}
              onUpload={async (file) => {
                if (!statementReady || recovery || manualRecovery)
                  throw new FormValidationError('请先明确保存恢复的题面，再上传附件。')
                if (file.size > 64 * 1024 * 1024)
                  throw new FormValidationError('附件不能超过 64 MiB。')
                if (savingLock.current) throw new FormValidationError('正在保存题面，请稍后重试。')
                const base = dirty ? await save() : copy
                if (!base) throw new FormValidationError('请先处理题面的保存错误。')
                savingLock.current = true
                setSaving(true)
                try {
                  const id = crypto.randomUUID()
                  const suffix = file.name.match(/\.[a-zA-Z0-9]{1,12}$/)?.[0].toLowerCase() ?? ''
                  const asset: DomainTreeEntry = {
                    id,
                    kind: 'asset',
                    path: availableLocation(`attachments/${id}${suffix}`, base.tree.entries, id),
                    attributes: { label: file.name, visibility: 'public' },
                    blob: await api.postApiAuthoringProblemsIdBlobs(problemId, { file }),
                  }
                  const next = await api.putApiAuthoringProblemsIdWorkingCopy(problemId, {
                    etag: base.etag,
                    tree: { entries: [...base.tree.entries, asset] },
                  })
                  onSaved(next)
                  return asset
                } finally {
                  savingLock.current = false
                  setSaving(false)
                }
              }}
            />
          ) : (
            <Textarea
              aria-label="数据内容"
              className="min-h-80 resize-none font-mono"
              value={text}
              disabled={!canEdit}
              onChange={(e) => changeText(e.target.value)}
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
