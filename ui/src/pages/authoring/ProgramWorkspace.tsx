import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type MutableRefObject,
} from 'react'
import {
  Check,
  Loader2,
  Code2,
  FilePlus2,
  MoreHorizontal,
  Plus,
  Settings2,
  Upload,
  Play,
  Trash2,
  Download,
} from 'lucide-react'
import type {
  DomainMaterialView,
  DomainProgramMaterial,
  DomainWorkingCopy,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import CodeEditor from '@/components/CodeEditor'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { entryLabel, roleNames } from '@/lib/authoring-materials'
import { apiError, formatFileSize } from '@/lib/format'
import { Choice, Field } from './FormFields'
import { programDirectory } from './material-locations'
import {
  newProgram,
  programLanguages,
  programRoles,
  saveProgramDraft,
  type ProgramDraft,
} from './program-draft'

export default function ProgramWorkspace({
  problemId,
  copy,
  revision,
  canEdit,
  onSaved,
  onDirty,
  onBusy,
  beforeLeave,
}: {
  problemId: string
  copy: DomainWorkingCopy
  revision?: number
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onDirty: (dirty: boolean) => void
  onBusy: (busy: boolean) => void
  beforeLeave: MutableRefObject<(() => Promise<boolean>) | null>
}) {
  const api = useDomainAPI(),
    confirm = useConfirm()
  const [programs, setPrograms] = useState<DomainMaterialView[]>([]),
    [selected, setSelected] = useState('')
  const [draft, setDraft] = useState<ProgramDraft>(),
    [texts, setTexts] = useState<Record<string, string>>({}),
    [activeSource, setActiveSource] = useState('')
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [dirty, setDirty] = useState(false),
    [error, setError] = useState('')
  const [dialog, setDialog] = useState<'create' | 'settings' | 'source' | null>(null),
    [formError, setFormError] = useState('')
  const [name, setName] = useState(''),
    [role, setRole] = useState('solution'),
    [language, setLanguage] = useState('cpp'),
    [filename, setFilename] = useState('helper.cpp')
  const [settings, setSettings] = useState<DomainProgramMaterial>()
  const fileInput = useRef<HTMLInputElement>(null),
    importInput = useRef<HTMLInputElement>(null),
    folderInput = useRef<HTMLInputElement>(null)
  const ownCopy = useRef<string | null>(null),
    lock = useRef(false),
    sequence = useRef(0),
    currentDraft = useRef(draft)
  currentDraft.current = draft
  const currentCopy = useRef(copy)
  currentCopy.current = copy
  const editVersion = useRef(0)
  const [reload, setReload] = useState(0)
  const assigned = new Set(programs.flatMap((p) => p.program?.files ?? []))
  const unassigned = copy.tree.entries.filter((e) => e.kind === 'source' && !assigned.has(e.id))
  const source =
    draft?.sources.find((e) => e.id === activeSource) ??
    draft?.sources.find((e) => e.id === draft.program.entryPoint) ??
    draft?.sources[0]
  const description =
    programRoles.find((r) => r.id === draft?.program.role)?.description ??
    '此程序保留导入时的用途与运行方式。'
  useEffect(() => {
    onDirty(dirty)
    return () => onDirty(false)
  }, [dirty, onDirty])
  useEffect(() => {
    onBusy(busy || loading)
    return () => onBusy(false)
  }, [busy, loading, onBusy])

  const loadProgram = useCallback(
    async (item: DomainMaterialView, base: DomainWorkingCopy) => {
      if (!item.program) throw new Error('此程序定义无法读取，请从版本管理恢复有效版本。')
      const sources = item.program.files.flatMap((id) => {
        const entry = base.tree.entries.find((e) => e.id === id)
        return entry ? [entry] : []
      })
      if (sources.length !== item.program.files.length)
        throw new Error('此程序有缺失的代码，请从版本管理恢复。')
      const content: Record<string, string> = {}
      let budget = 0
      for (const entry of sources) {
        if (entry.blob.bytes > 1024 * 1024 || (budget += entry.blob.bytes) > 8 * 1024 * 1024)
          continue
        const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256)
        try {
          const value = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
          if (!value.includes('\0')) content[entry.id] = value
        } catch {
          /* Preserve binary companions for download and replacement. */
        }
      }
      return {
        draft: { entry: item.entry, program: item.program, sources, changes: {} } as ProgramDraft,
        content,
      }
    },
    [api, problemId],
  )
  useEffect(() => {
    if (copy.etag === ownCopy.current) return
    if (dirty) {
      setError('副本有新变化，本地输入已保留。请先保存或重新读取。')
      return
    }
    const generation = ++sequence.current
    setLoading(true)
    setError('')
    void (async () => {
      const items: DomainMaterialView[] = []
      let after: string | undefined
      do {
        const page = await api.getApiAuthoringProblemsIdMaterials(problemId, {
          kind: 'program',
          limit: 100,
          after,
          ...(revision ? { revision } : { etag: copy.etag }),
        })
        items.push(...page.items)
        after = page.next || undefined
      } while (after)
      const item = items.find((p) => p.entry.id === selected) ?? items[0]
      const result = item ? await loadProgram(item, copy) : undefined
      if (generation !== sequence.current) return
      setPrograms(items)
      setSelected(item?.entry.id ?? '')
      setDraft(result?.draft)
      setTexts(result?.content ?? {})
      setActiveSource(item?.program?.entryPoint ?? '')
    })()
      .catch((e) => {
        if (generation === sequence.current) setError(apiError(e, '程序加载失败'))
      })
      .finally(() => {
        if (generation === sequence.current) setLoading(false)
      })
    return () => {
      sequence.current++
    }
    // A changed copy is the reload boundary; selection is loaded separately.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, problemId, copy.etag, revision, loadProgram, reload])
  async function reloadCopy() {
    if (
      dirty &&
      !(await confirm({
        title: '重新读取服务器上的程序？',
        description: '本页未保存的代码会被丢弃。需要保留时，可先通过“当前代码操作”下载。',
        confirmLabel: '丢弃本地输入并重新读取',
        destructive: true,
      }))
    )
      return
    setBusy(true)
    try {
      const latest = canEdit ? await api.getApiAuthoringProblemsIdWorkingCopy(problemId) : copy
      ownCopy.current = null
      setDirty(false)
      setError('')
      onSaved(latest)
      setReload((n) => n + 1)
    } catch (e) {
      setError(apiError(e, '重新读取失败，本地输入仍保留'))
    } finally {
      setBusy(false)
    }
  }
  function change(next: ProgramDraft) {
    editVersion.current++
    setDraft(next)
    currentDraft.current = next
    setDirty(true)

    setError('')
  }
  const save = useCallback(async () => {
    const snapshot = currentDraft.current
    if (!snapshot || !canEdit || lock.current) return false
    lock.current = true
    setBusy(true)
    setError('')
    const version = editVersion.current
    try {
      const result = await saveProgramDraft(
        api,
        problemId,
        currentCopy.current,
        snapshot,
        programs
          .filter((p) => p.entry.id !== snapshot.entry.id)
          .flatMap((p) => (p.program ? [p.program] : [])),
      )
      const entry = result.copy.tree.entries.find((e) => e.id === snapshot.entry.id)!
      ownCopy.current = result.copy.etag
      currentCopy.current = result.copy
      const updated = { entry, program: result.program, sources: result.sources, changes: {} }
      if (version === editVersion.current) {
        setDraft(updated)
        currentDraft.current = updated
        setDirty(false)
      } else {
        const pending = currentDraft.current!
        const next = {
          ...updated,
          changes: Object.fromEntries(
            Object.entries(pending.changes)
              .filter(([id, blob]) => blob !== snapshot.changes[id])
              .map(([id, blob]) => [result.remap.get(id) ?? id, blob]),
          ),
        }
        setDraft(next)
        currentDraft.current = next
      }
      setTexts((prev) =>
        Object.fromEntries(
          Object.entries(prev).map(([id, value]) => [result.remap.get(id) ?? id, value]),
        ),
      )
      setActiveSource((id) => result.remap.get(id) ?? id)
      setPrograms((items) => {
        const next = { entry, program: result.program, position: 0 }
        return items.some((p) => p.entry.id === entry.id)
          ? items.map((p) => (p.entry.id === entry.id ? next : p))
          : [...items, next]
      })
      onSaved(result.copy)
      return true
    } catch (e) {
      setError(apiError(e, '保存失败，代码仍保留在本页'))
      return false
    } finally {
      lock.current = false
      setBusy(false)
    }
  }, [api, problemId, canEdit, programs, onSaved])
  useLayoutEffect(() => {
    beforeLeave.current = async () => !dirty || (await save())
    return () => {
      beforeLeave.current = null
    }
  }, [beforeLeave, dirty, save])
  useEffect(() => {
    if (!dirty || busy || error || dialog === 'settings') return
    const timer = setTimeout(() => void save(), 1000)
    return () => clearTimeout(timer)
  }, [dirty, busy, error, save, dialog])
  async function openProgram(item: DomainMaterialView) {
    if (item.entry.id === selected || busy || (dirty && !(await save()))) return
    setLoading(true)
    setError('')
    try {
      const result = await loadProgram(item, currentCopy.current)
      setSelected(item.entry.id)
      setDraft(result.draft)
      setTexts(result.content)
      setActiveSource(result.draft.program.entryPoint)
      setDirty(false)
    } catch (e) {
      setError(apiError(e, '打开程序失败'))
    } finally {
      setLoading(false)
    }
  }
  async function install(next: ProgramDraft) {
    setBusy(true)
    setFormError('')
    lock.current = true
    let installed = false
    try {
      const result = await saveProgramDraft(
        api,
        problemId,
        currentCopy.current,
        next,
        programs.flatMap((p) => (p.program ? [p.program] : [])),
      )
      ownCopy.current = result.copy.etag
      currentCopy.current = result.copy
      const item = {
        entry: result.copy.tree.entries.find((e) => e.id === next.entry.id)!,
        program: result.program,
        position: programs.length,
      }
      installed = true
      onSaved(result.copy)
      setPrograms((p) => [...p, item])
      setSelected(item.entry.id)
      setDirty(false)
      setDialog(null)
      setDraft(undefined)
      const loaded = await loadProgram(item, result.copy)
      setDraft(loaded.draft)
      setTexts(loaded.content)
      setActiveSource(result.program.entryPoint)
    } catch (e) {
      const message = installed
        ? '程序已保存，但代码未能加载。请重新读取，无需重复创建。'
        : apiError(e, '创建程序失败')
      setFormError(message)
      setError(message)
    } finally {
      setBusy(false)
      lock.current = false
    }
  }
  async function create() {
    if (!name.trim()) {
      setFormError('请为程序取一个能辨认用途的名称。')
      return
    }
    await install(newProgram(name.trim(), role, language))
  }
  async function importProgram(files: File[]) {
    if (!files.length || busy || (dirty && !(await save()))) return
    setError('')
    try {
      if (files.length > 100 || files.some((f) => f.size > 64 * 1024 * 1024))
        throw new Error('一次最多导入 100 份代码，每份不超过 64 MiB。')
      const primary =
        files.find((f) => /^main\.(cpp|cc|cxx|c|py|rs|java)$/i.test(f.name)) ??
        files.find((f) => /\.(cpp|cc|cxx|c|py|rs|java)$/i.test(f.name))
      if (!primary) throw new Error('请包含一个可运行的 C++、C、Python、Java 或 Rust 程序。')
      const lang = /\.py$/i.test(primary.name)
        ? 'python'
        : /\.java$/i.test(primary.name)
          ? 'java'
          : /\.rs$/i.test(primary.name)
            ? 'rust'
            : /\.c$/i.test(primary.name)
              ? 'c'
              : 'cpp'
      const next = newProgram(primary.name.replace(/\.[^.]+$/, ''), 'solution', lang),
        names = new Set<string>()
      next.changes = {}
      next.sources = files.map((file) => {
        const relative = file.webkitRelativePath || file.name
        if (names.has(relative.toLowerCase())) throw new Error('同一次导入有同名代码，请分开导入。')
        names.add(relative.toLowerCase())
        const id = crypto.randomUUID()
        next.changes[id] = file
        if (file === primary) next.program.entryPoint = id
        return {
          id,
          kind: 'source',
          path: `${next.program.directory}/${relative}`,
          attributes: { label: file.name, language: lang },
          blob: { sha256: '', bytes: 0 },
        }
      })
      next.program.files = next.sources.map((e) => e.id)
      next.program.directory = programDirectory(next.program.files, next.sources)
      await install(next)
    } catch (e) {
      setError(apiError(e, '导入程序失败'))
    } finally {
      if (importInput.current) importInput.current.value = ''
      if (folderInput.current) folderInput.current.value = ''
    }
  }
  async function addSource(file?: File) {
    if (!draft) return
    const label = file?.name ?? filename.trim()
    if (!label || /[\\/\x00-\x1f]/.test(label) || label === '.' || label === '..') {
      setFormError('请填写代码名称，例如 helper.cpp。')
      return
    }
    if (draft.sources.some((e) => e.path.split('/').pop()?.toLowerCase() === label.toLowerCase())) {
      setFormError('程序内已有同名代码，请打开已有标签编辑。')
      return
    }
    if (file && file.size > 64 * 1024 * 1024) {
      setFormError('辅助文件不能超过 64 MiB。')
      return
    }
    const id = crypto.randomUUID(),
      primary = draft.sources.find((e) => e.id === draft.program.entryPoint)
    const directory = primary?.path.split('/').slice(0, -1).join('/') || draft.program.directory
    // Keep the requested basename for #include/import. The save transaction
    // isolates the entire layout if an older material occupies this location.
    const path = `${directory ? directory + '/' : ''}${label}`
    const entry = {
      id,
      kind: 'source',
      path,
      attributes: { label, language: draft.program.language },
      blob: { sha256: '', bytes: 0 },
    }
    const blob = file ?? new Blob([''])
    if (blob.size <= 1024 * 1024) {
      try {
        const value = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
        if (!value.includes('\0')) setTexts((p) => ({ ...p, [id]: value }))
      } catch {
        /* binary companion remains downloadable */
      }
    }
    change({
      ...draft,
      program: { ...draft.program, files: [...draft.program.files, id] },
      sources: [...draft.sources, entry],
      changes: { ...draft.changes, [id]: blob },
    })
    setActiveSource(id)
    setDialog(null)
  }
  async function download() {
    if (!source || !draft) return
    try {
      const blob =
        draft.changes[source.id] ??
        (await api.getApiAuthoringProblemsIdBlobsDigest(problemId, source.blob.sha256))
      const url = URL.createObjectURL(blob),
        a = document.createElement('a')
      a.href = url
      a.download = source.path.split('/').pop()!
      a.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(apiError(e, '下载失败'))
    }
  }
  async function removeProgram() {
    if (
      !draft ||
      !(await confirm({
        title: `移除“${draft.program.name}”？`,
        description: '已有版本不受影响。测试或判定规则若在使用此程序，需要另选一个程序。',
        confirmLabel: '移除程序',
        destructive: true,
      }))
    )
      return
    setBusy(true)
    try {
      const next = await api.deleteApiAuthoringProblemsIdWorkingCopyEntriesEntryId(
        problemId,
        draft.entry.id,
        { etag: copy.etag },
      )
      setDirty(false)
      onSaved(next)
    } catch (e) {
      setError(apiError(e, '移除失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">程序</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            编写参考解、生成数据，或检查输入与答案。
          </p>
        </div>
        {canEdit && (
          <div className="flex gap-2">
            <input
              ref={importInput}
              type="file"
              multiple
              className="hidden"
              aria-label="导入程序代码"
              onChange={(e) => void importProgram([...(e.target.files ?? [])])}
            />
            <input
              ref={folderInput}
              type="file"
              multiple
              {...{ webkitdirectory: '' }}
              className="hidden"
              onChange={(e) => void importProgram([...(e.target.files ?? [])])}
            />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" disabled={busy || loading || dirty}>
                  <Upload />
                  导入程序
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                <DropdownMenuItem onSelect={() => importInput.current?.click()}>
                  选择代码文件
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => folderInput.current?.click()}>
                  导入多文件程序
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              disabled={busy || loading || dirty}
              onClick={() => {
                setName('')
                setRole('solution')
                setLanguage('cpp')
                setFormError('')
                setDialog('create')
              }}
            >
              <Plus />
              新建程序
            </Button>
          </div>
        )}
      </div>
      {error && (
        <div
          role="alert"
          className="rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
        >
          <p>{error}</p>
          <div className="mt-3 flex gap-2">
            {dirty && canEdit && (
              <Button size="sm" variant="outline" disabled={busy} onClick={() => void save()}>
                重试保存
              </Button>
            )}
            <Button size="sm" variant="outline" disabled={busy} onClick={() => void reloadCopy()}>
              重新读取
            </Button>
          </div>
        </div>
      )}
      <div className="grid min-w-0 gap-4 xl:grid-cols-[200px_minmax(0,1fr)]">
        <nav aria-label="题目程序" className="flex gap-2 overflow-x-auto xl:block xl:space-y-2">
          {programs.map((item) => (
            <button
              key={item.entry.id}
              type="button"
              disabled={busy || loading}
              aria-current={selected === item.entry.id ? 'page' : undefined}
              onClick={() => void openProgram(item)}
              className={`flex min-w-40 items-start gap-2 rounded-lg border px-3 py-3 text-left transition-colors xl:w-full ${selected === item.entry.id ? 'border-primary/40 bg-primary/10' : 'border-transparent hover:bg-muted'}`}
            >
              <Code2
                className={`mt-0.5 size-4 shrink-0 ${selected === item.entry.id ? 'text-primary' : 'text-muted-foreground'}`}
              />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">
                  {item.program?.name ?? entryLabel(item.entry)}
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  {roleNames[item.program?.role ?? ''] ?? '待修复'} ·{' '}
                  {programLanguages.find(([id]) => id === item.program?.language)?.[1] ??
                    item.program?.language}
                </span>
              </span>
              {selected === item.entry.id && (
                <Check className="mt-0.5 size-4 shrink-0 text-primary" />
              )}
            </button>
          ))}
        </nav>
        {loading ? (
          <p className="py-12 text-center text-sm text-muted-foreground">正在打开程序…</p>
        ) : draft ? (
          <section
            className="min-w-0 overflow-hidden rounded-xl border bg-card"
            aria-label="程序编辑区"
          >
            <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
              <div className="min-w-0">
                <h3 className="truncate text-base font-semibold">{draft.program.name}</h3>
                <p className="mt-1 max-w-xl text-xs leading-5 text-muted-foreground">
                  {description}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <span
                  role="status"
                  className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
                >
                  {busy ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : !dirty ? (
                    <Check className="size-3.5" />
                  ) : null}
                  {busy
                    ? '正在保存…'
                    : dirty
                      ? '有未保存的更改'
                      : canEdit
                        ? '草稿已保存'
                        : '只读版本'}
                </span>
                <Button
                  variant="outline"
                  size="icon"
                  aria-label="程序设置"
                  disabled={busy}
                  onClick={() => {
                    setSettings(structuredClone(draft.program))
                    setFormError('')
                    setDialog('settings')
                  }}
                >
                  <Settings2 />
                </Button>
              </div>
            </div>
            <div className="flex items-center gap-1 border-b bg-muted/15 px-3">
              <div
                className="flex min-w-0 flex-1 overflow-x-auto"
                role="tablist"
                aria-label="程序代码"
              >
                {draft.sources.map((file, index) => (
                  <button
                    key={file.id}
                    type="button"
                    role="tab"
                    aria-selected={source?.id === file.id}
                    onClick={() => setActiveSource(file.id)}
                    className={`flex shrink-0 items-center gap-2 border-b-2 px-3 py-3 text-sm transition-colors ${source?.id === file.id ? 'border-primary bg-primary/5 font-medium text-primary' : 'border-transparent text-muted-foreground hover:bg-muted'}`}
                  >
                    {entryLabel(file)}
                    {draft.program.entryPoint === file.id ? (
                      <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px]">
                        主代码
                      </span>
                    ) : (
                      <span className="text-[10px] text-muted-foreground">辅助 {index + 1}</span>
                    )}
                  </button>
                ))}
              </div>
              {canEdit && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="添加辅助代码"
                  title="添加辅助代码"
                  disabled={busy || loading}
                  onClick={() => {
                    setFilename(draft.program.language === 'python' ? 'helper.py' : 'helper.cpp')
                    setFormError('')
                    setDialog('source')
                  }}
                >
                  <FilePlus2 />
                </Button>
              )}
              {source && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" aria-label="当前代码操作">
                      <MoreHorizontal />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    {canEdit && (
                      <DropdownMenuItem disabled={!dirty || busy} onSelect={() => void save()}>
                        立即保存
                      </DropdownMenuItem>
                    )}
                    <DropdownMenuItem onSelect={() => void download()}>
                      <Download />
                      下载这份代码
                    </DropdownMenuItem>
                    {canEdit && source.id !== draft.program.entryPoint && (
                      <>
                        <DropdownMenuItem
                          disabled={busy}
                          onSelect={() =>
                            change({
                              ...draft,
                              program: { ...draft.program, entryPoint: source.id },
                            })
                          }
                        >
                          <Play />
                          设为主代码
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          disabled={busy}
                          onSelect={async () => {
                            if (
                              !(await confirm({
                                title: '从此程序移除辅助代码？',
                                description: '其他程序和历史版本不受影响。',
                                confirmLabel: '移除辅助代码',
                                destructive: true,
                              }))
                            )
                              return
                            change({
                              ...draft,
                              program: {
                                ...draft.program,
                                files: draft.program.files.filter((id) => id !== source.id),
                              },
                              sources: draft.sources.filter((e) => e.id !== source.id),
                            })
                            setActiveSource(draft.program.entryPoint)
                          }}
                        >
                          <Trash2 />
                          移出此程序
                        </DropdownMenuItem>
                      </>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>
            {source && texts[source.id] !== undefined ? (
              <div className="h-[max(380px,60dvh)] max-h-[850px]">
                <CodeEditor
                  language={draft.program.language}
                  documentKey={`${draft.entry.id}:${source.id}`}
                  value={texts[source.id]}
                  ariaLabel="程序代码编辑器"
                  readOnly={!canEdit}
                  className="rounded-none border-0"
                  onChange={(text) => {
                    setTexts((p) => ({ ...p, [source.id]: text }))
                    change({
                      ...currentDraft.current!,
                      changes: {
                        ...currentDraft.current!.changes,
                        [source.id]: new Blob([text], { type: 'text/plain' }),
                      },
                    })
                  }}
                />
              </div>
            ) : source ? (
              <div className="p-8 text-center text-sm text-muted-foreground">
                {formatFileSize(source.blob.bytes)} · 此辅助文件保留原始内容。
                <Button className="ml-3" variant="outline" onClick={() => void download()}>
                  下载查看
                </Button>
              </div>
            ) : (
              <div className="p-8 text-sm text-muted-foreground">
                这个程序还没有代码，点击右上角添加代码。
              </div>
            )}
            <div className="flex flex-wrap justify-between gap-2 border-t px-4 py-2 text-xs text-muted-foreground">
              <span>
                {programLanguages.find(([id]) => id === draft.program.language)?.[1] ??
                  draft.program.language}{' '}
                ·{' '}
                {draft.sources.length === 1
                  ? '单文件程序'
                  : `${draft.sources.length} 份代码组成一个程序`}
              </span>
              <span>
                {draft.program.role === 'solution'
                  ? `预期：${draft.program.expectedVerdicts.map((v) => ({ Accepted: '通过全部测试', 'Wrong Answer': '答案错误', 'Time Limit Exceeded': '超时', 'Runtime Error': '运行错误', 'Any Rejection': '任意不通过' })[v] ?? v).join(' / ')}`
                  : '通过“检查与发布”运行检查'}
              </span>
            </div>
          </section>
        ) : (
          <div className="rounded-xl border border-dashed p-10 text-center">
            <Code2 className="mx-auto mb-4 size-8 text-muted-foreground" />
            <h3 className="font-medium">先写一份参考解</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              新建程序后直接编写代码，也可以导入现有程序。
            </p>
          </div>
        )}
      </div>
      {canEdit && unassigned.length > 0 && (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">
            整理旧资料 · {unassigned.length} 份代码尚未归属程序
          </summary>
          <p className="my-3">这些是旧版本或题包保留的代码。可直接转为程序，原有内容不会丢失。</p>
          <div className="flex flex-wrap gap-2">
            {unassigned.map((e) => (
              <Button
                key={e.id}
                size="sm"
                variant="outline"
                disabled={busy || dirty}
                onClick={() => {
                  const next = newProgram(entryLabel(e), 'solution', e.attributes.language || 'cpp')
                  next.sources = [e]
                  next.changes = {}
                  next.program.files = [e.id]
                  next.program.entryPoint = e.id
                  next.program.directory = programDirectory([e.id], [e])
                  void install(next)
                }}
              >
                {entryLabel(e)} · 创建程序
              </Button>
            ))}
          </div>
        </details>
      )}
      <Dialog
        open={dialog !== null}
        onOpenChange={(open) => {
          if (!open && !busy) setDialog(null)
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogTitle>
            {dialog === 'create' ? '新建程序' : dialog === 'source' ? '添加辅助代码' : '程序设置'}
          </DialogTitle>
          <DialogDescription>
            {dialog === 'create'
              ? '选择用途后直接开始编写，系统自动准备主代码。'
              : dialog === 'source'
                ? '辅助代码会自动加入当前程序，无需再关联。'
                : '调整程序用途和检查时的预期行为。'}
          </DialogDescription>
          {dialog === 'create' ? (
            <>
              <Field label="程序名称">
                <Input
                  aria-label="程序名称"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="例如：标准解 · 最短路"
                />
              </Field>
              <Choice
                label="编程语言"
                value={language}
                onChange={setLanguage}
                options={programLanguages}
              />
              <RoleChoices value={role} onChange={setRole} />
              <div className="flex justify-end">
                <Button loading={busy} onClick={() => void create()}>
                  创建并开始编写
                </Button>
              </div>
            </>
          ) : dialog === 'source' ? (
            <>
              <Field label="代码名称" hint="例如 helper.cpp 或 utils.py，后续可在主代码中引用。">
                <Input
                  aria-label="辅助代码名称"
                  value={filename}
                  onChange={(e) => setFilename(e.target.value)}
                />
              </Field>
              <input
                ref={fileInput}
                className="hidden"
                type="file"
                aria-label="上传辅助代码"
                onChange={(e) => {
                  if (e.target.files?.[0]) void addSource(e.target.files[0])
                }}
              />
              <div className="flex justify-between">
                <Button variant="outline" onClick={() => fileInput.current?.click()}>
                  <Upload />
                  上传现有代码
                </Button>
                <Button onClick={() => void addSource()}>添加并编辑</Button>
              </div>
            </>
          ) : (
            settings && (
              <>
                <Field label="程序名称">
                  <Input
                    aria-label="程序名称"
                    disabled={!canEdit}
                    value={settings.name}
                    onChange={(e) => setSettings({ ...settings, name: e.target.value })}
                  />
                </Field>
                <RoleChoices
                  disabled={!canEdit}
                  value={settings.role}
                  onChange={(role) =>
                    setSettings({
                      ...settings,
                      role,
                      expectedVerdicts: role === 'solution' ? ['Accepted'] : [],
                      protocol: 'stdio',
                    })
                  }
                />
                {settings.role === 'solution' && (
                  <fieldset className="space-y-3">
                    <legend className="mb-2 text-sm font-medium">这份解答应当得到什么结果？</legend>
                    <div className="grid gap-2 sm:grid-cols-2">
                      {[
                        ['Accepted', '通过全部测试'],
                        ['Wrong Answer', '答案错误'],
                        ['Time Limit Exceeded', '超时'],
                        ['Runtime Error', '运行错误'],
                        ['Any Rejection', '任意不通过'],
                      ].map(([id, label]) => (
                        <label
                          key={id}
                          className={`flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-3 text-sm ${settings.expectedVerdicts.includes(id) ? 'border-primary/50 bg-primary/10' : 'border-border'}`}
                        >
                          <input
                            type="checkbox"
                            className="size-4 accent-primary"
                            disabled={!canEdit}
                            checked={settings.expectedVerdicts.includes(id)}
                            onChange={() =>
                              setSettings({
                                ...settings,
                                expectedVerdicts: settings.expectedVerdicts.includes(id)
                                  ? settings.expectedVerdicts.filter((v) => v !== id)
                                  : id === 'Accepted'
                                    ? ['Accepted']
                                    : [
                                        ...settings.expectedVerdicts.filter(
                                          (v) => v !== 'Accepted',
                                        ),
                                        id,
                                      ],
                              })
                            }
                          />
                          {label}
                        </label>
                      ))}
                    </div>
                    <p className="text-xs leading-5 text-muted-foreground">
                      正确参考解应通过全部测试；放入错误解可以检查数据是否足够强。
                    </p>
                  </fieldset>
                )}
                <details className="rounded-lg border p-3">
                  <summary className="cursor-pointer text-sm">运行选项</summary>
                  <div className="mt-4 space-y-4">
                    <Choice
                      disabled={!canEdit}
                      label="编程语言"
                      value={settings.language}
                      onChange={(language) => setSettings({ ...settings, language })}
                      options={programLanguages}
                    />
                    <Choice
                      disabled={!canEdit}
                      label="运行协议"
                      value={settings.protocol}
                      onChange={(protocol) => setSettings({ ...settings, protocol })}
                      options={[
                        ['stdio', '标准输入输出'],
                        ['testlib', 'testlib'],
                        ['kattis', 'Kattis'],
                      ]}
                    />
                    <Field label="运行参数" hint="每行一个参数。">
                      <Textarea
                        aria-label="运行参数"
                        className="resize-none"
                        disabled={!canEdit}
                        value={settings.arguments.join('\n')}
                        onChange={(e) =>
                          setSettings({
                            ...settings,
                            arguments: e.target.value ? e.target.value.split('\n') : [],
                          })
                        }
                      />
                    </Field>
                  </div>
                </details>
                {canEdit && (
                  <div className="flex justify-between">
                    <Button
                      variant="ghost"
                      disabled={busy || dirty}
                      onClick={() => {
                        setDialog(null)
                        void removeProgram()
                      }}
                    >
                      移除程序
                    </Button>
                    <Button
                      disabled={busy}
                      onClick={() => {
                        if (
                          !settings.name.trim() ||
                          (settings.role === 'solution' && !settings.expectedVerdicts.length)
                        ) {
                          setFormError('请填写名称，并选择至少一种预期结果。')
                          return
                        }
                        change({ ...draft!, program: { ...settings, name: settings.name.trim() } })
                        setDialog(null)
                      }}
                    >
                      应用设置
                    </Button>
                  </div>
                )}
              </>
            )
          )}
          {formError && (
            <p role="alert" className="text-sm text-destructive">
              {formError}
            </p>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}

function RoleChoices({
  value,
  onChange,
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}) {
  return (
    <fieldset>
      <legend className="mb-2 text-sm font-medium">程序用途</legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {programRoles.map((role) => (
          <label
            key={role.id}
            className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 ${value === role.id ? 'border-primary/50 bg-primary/10' : 'border-border hover:bg-muted/30'}`}
          >
            <input
              type="radio"
              name="program-role"
              className="mt-1 size-4 shrink-0 accent-primary"
              checked={value === role.id}
              disabled={disabled}
              onChange={() => onChange(role.id)}
            />
            <span>
              <span className="block text-sm font-medium">{role.name}</span>
              <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                {role.description}
              </span>
            </span>
          </label>
        ))}
      </div>
      {!programRoles.some((r) => r.id === value) && (
        <p className="mt-2 text-sm">导入的用途：{roleNames[value] ?? value}</p>
      )}
    </fieldset>
  )
}
