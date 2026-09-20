import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  BookOpen,
  Code2,
  Database,
  Files,
  GitCommitHorizontal,
  History,
  Menu,
  Plus,
  Settings2,
  Users,
  ArrowDownToLine,
  ArrowUpFromLine,
  Package,
  CheckCheck,
  Upload,
} from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import { Link, useNavigate } from '@/domain/navigation'
import { useAuth } from '@/auth/AuthContext'
import type { DomainWorkingCopy, DomainTreeEntry, DtoProblemResponse } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { useConfirm } from '@/components/ui/confirm-dialog'
import ResourceCollaboration from '@/components/ResourceCollaboration'
import { apiError, FormValidationError } from '@/lib/format'
import {
  defaultProgram,
  defaultTest,
  defaultGroup,
  entryLabel,
  materialNames,
} from '@/lib/authoring-materials'
import MaterialEditor from './MaterialEditor'
import ChangesPanel from './ChangesPanel'
import PackagesPanel from './PackagesPanel'
import ChecksPanel from './ChecksPanel'
import ReleasesPanel from './ReleasesPanel'
import MaterialBrowser from './MaterialBrowser'
import ResourcePolicyPanel from './ResourcePolicyPanel'
import { Choice, Field } from './MaterialForm'

const sections = [
  { id: 'overview', label: '基本设置', icon: Settings2, kinds: ['metadata'] },
  { id: 'statement', label: '题面', icon: BookOpen, kinds: ['statement'] },
  { id: 'programs', label: '程序', icon: Code2, kinds: ['program', 'source'] },
  {
    id: 'tests',
    label: '测试数据',
    icon: Database,
    kinds: ['test', 'validation', 'group', 'input', 'answer'],
  },
  { id: 'assets', label: '附件', icon: Files, kinds: ['asset', 'resource'] },
  { id: 'changes', label: '更改与提交', icon: GitCommitHorizontal, kinds: [] },
  { id: 'history', label: '提交历史', icon: History, kinds: [] },
  { id: 'checks', label: '检查', icon: CheckCheck, kinds: [] },
  { id: 'releases', label: '发布', icon: Upload, kinds: [] },
  { id: 'packages', label: '题包', icon: Package, kinds: [] },
  { id: 'collaboration', label: '协作与权限', icon: Users, kinds: [] },
]

export default function WorkbenchPage() {
  const { id = '', section = 'statement' } = useParams(),
    { slug } = useDomain(),
    { user } = useAuth()
  return <Workbench key={`${slug}:${user?.id}:${id}`} id={id} section={section} />
}

function Workbench({ id, section }: { id: string; section: string }) {
  const api = useDomainAPI(),
    navigate = useNavigate(),
    confirm = useConfirm()
  const [problem, setProblem] = useState<DtoProblemResponse>(),
    [copy, setCopy] = useState<DomainWorkingCopy>(),
    [title, setTitle] = useState(''),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(true),
    [operation, setOperation] = useState<'update' | 'create' | 'upload' | null>(null)
  const busy = operation !== null
  const [uploadProgress, setUploadProgress] = useState({ done: 0, total: 0 }),
    [uploadNotice, setUploadNotice] = useState('')
  const [dirty, setDirty] = useState(false),
    [editorBusy, setEditorBusy] = useState(false),
    [selected, setSelected] = useState(''),
    [drawer, setDrawer] = useState(false),
    [creating, setCreating] = useState(false),
    [kind, setKind] = useState('source'),
    [name, setName] = useState(''),
    [language, setLanguage] = useState('zh'),
    [filePath, setFilePath] = useState(''),
    [createError, setCreateError] = useState('')
  const [revision, setRevision] = useState<number>(),
    generation = useRef(0),
    fileInput = useRef<HTMLInputElement>(null),
    folderInput = useRef<HTMLInputElement>(null),
    materialTitle = useRef<HTMLHeadingElement>(null),
    drawerTitle = useRef<HTMLParagraphElement>(null)
  const current = sections.find((item) => item.id === section)
  const canEdit = Boolean(problem?.permissions.edit)
  const load = useCallback(async () => {
    const n = ++generation.current
    setLoading(true)
    setError('')
    try {
      const problem = await api.getApiAdminProblemsId(id)
      if (n !== generation.current) return
      setProblem(problem)
      setTitle(problem.title)
      if (problem.permissions.edit) {
        const value = await api.postApiAuthoringProblemsIdWorkingCopy(id)
        if (n === generation.current) setCopy(value)
      } else {
        const history = await api.getApiAuthoringProblemsIdCommits(id, { limit: 1 })
        if (n !== generation.current) return
        const last = history.items[0]
        if (last) {
          const value = await api.getApiAuthoringProblemsIdCommitsRevision(id, last.revision)
          if (n === generation.current) {
            setRevision(last.revision)
            setCopy({
              tree: value.tree,
              etag: '',
              baseRevision: last.revision,
              headRevision: last.revision,
              updatedAt: last.createdAt,
            })
          }
        }
      }
    } catch (error) {
      if (n === generation.current) setError(apiError(error, '工作台加载失败'))
    } finally {
      if (n === generation.current) setLoading(false)
    }
  }, [api, id])
  useEffect(() => {
    void load()
    return () => {
      generation.current++
    }
  }, [load])
  const saved = useCallback((value: DomainWorkingCopy) => {
    setCopy(value)
    setError('')
  }, [])
  const metadataHash = copy?.tree.entries.find((entry) => entry.id === 'problem')?.blob.sha256
  useEffect(() => {
    if (!metadataHash) return
    let active = true
    api
      .getApiAuthoringProblemsIdMaterialsEntryId(id, 'problem', revision ? { revision } : undefined)
      .then((value) => {
        if (active && value.metadata) setTitle(value.metadata.title)
      })
      .catch(() => {})
    return () => {
      active = false
    }
  }, [api, id, metadataHash, revision])
  // Protect failed or unsaved edits on both full-page exits and ordinary site
  // links. Confirmation uses the project's dialog, not a browser alert.
  useEffect(() => {
    if (!dirty) return
    const unload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    const click = (event: MouseEvent) => {
      const target =
        event.target instanceof Element
          ? (event.target.closest('a[href]') as HTMLAnchorElement | null)
          : null
      if (
        !target ||
        event.defaultPrevented ||
        event.button !== 0 ||
        event.ctrlKey ||
        event.metaKey ||
        target.target === '_blank'
      )
        return
      const url = new URL(target.href)
      if (url.pathname === location.pathname && url.search === location.search) return
      event.preventDefault()
      event.stopPropagation()
      void confirm({
        title: '离开未保存的编辑？',
        description: '当前输入尚未保存到工作副本。',
        confirmLabel: '放弃本次输入',
        destructive: true,
      }).then((ok) => {
        if (ok) location.assign(url.href)
      })
    }
    window.addEventListener('beforeunload', unload)
    document.addEventListener('click', click, true)
    return () => {
      window.removeEventListener('beforeunload', unload)
      document.removeEventListener('click', click, true)
    }
  }, [dirty, confirm])
  async function chooseSection(value: string) {
    if (editorBusy || busy) return
    if (
      dirty &&
      !(await confirm({
        title: '切换前放弃未保存的输入？',
        description: '已保存的工作副本和历史提交不受影响。',
        confirmLabel: '放弃输入并切换',
        destructive: true,
      }))
    )
      return
    setDirty(false)
    setSelected('')
    setDrawer(false)
    setUploadNotice('')
    setError('')
    navigate(`/authoring/${id}/${value}`)
  }
  async function chooseEntry(entry: DomainTreeEntry) {
    if (editorBusy || busy) return
    if (
      dirty &&
      !(await confirm({
        title: '放弃当前未保存的输入？',
        description: '这次输入尚未保存，已保存内容不受影响。',
        confirmLabel: '切换材料',
        destructive: true,
      }))
    )
      return
    setDirty(false)
    setSelected(entry.id)
  }
  async function update() {
    if (!copy) return
    setOperation('update')
    setError('')
    try {
      const value = await api.postApiAuthoringProblemsIdWorkingCopyUpdate(id, { etag: copy.etag })
      saved(value.copy)
      if (value.merge) navigate(`/authoring/${id}/changes`)
    } catch (error) {
      setError(apiError(error, '更新失败，工作副本已保留'))
    } finally {
      setOperation(null)
    }
  }
  function startCreate() {
    setError('')
    setKind(current?.kinds.find((kind) => kind !== 'metadata') || 'source')
    setName('')
    setFilePath('')
    setLanguage('zh')
    setCreateError('')
    setCreating(true)
  }
  async function create() {
    if (!copy) return
    setOperation('create')
    setCreateError('')
    try {
      const entryId = crypto.randomUUID(),
        label = name.trim() || materialNames[kind]
      let text = '',
        path = filePath.trim(),
        attributes: Record<string, string> = {}
      if (kind === 'statement') {
        if (!/^[a-z]{2}(-[A-Za-z]{2,8})?$/.test(language))
          throw new FormValidationError('语言标识应为 zh、en 等格式')
        path = path || `statement/problem.${language}.md`
        attributes = { format: 'markdown', language }
        text = `# ${title}\n\n## 题目描述\n\n## 输入格式\n\n## 输出格式\n`
      } else if (kind === 'program') {
        path =
          path ||
          `vertex/programs/program-${copy.tree.entries.filter((e) => e.kind === 'program').length + 1}.json`
        text = JSON.stringify({ ...defaultProgram(), name: label })
        attributes = { format: 'json' }
      } else if (kind === 'test') {
        path =
          path ||
          `vertex/tests/test-${copy.tree.entries.filter((e) => e.kind === 'test').length + 1}.json`
        text = JSON.stringify({ ...defaultTest(), name: label })
        attributes = { format: 'json' }
      } else if (kind === 'validation') {
        path =
          filePath.trim() ||
          `vertex/validation/case-${copy.tree.entries.filter((e) => e.kind === 'validation').length + 1}.json`
        text = JSON.stringify({
          schemaVersion: 1,
          name: label,
          mode: 'invalid_output',
          input: '',
          answer: '',
          output: '',
          description: '',
        })
        attributes = { format: 'json' }
      } else if (kind === 'group') {
        path =
          path ||
          `vertex/groups/group-${copy.tree.entries.filter((e) => e.kind === 'group').length + 1}.json`
        text = JSON.stringify({ ...defaultGroup(), name: label })
        attributes = { format: 'json' }
      } else if (kind === 'source') {
        path =
          path ||
          `sources/solution-${copy.tree.entries.filter((e) => e.kind === 'source').length + 1}.cpp`
        text =
          '#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n    return 0;\n}\n'
        attributes = { language: 'cpp', format: 'text' }
      } else {
        path = path || `data/${kind === 'answer' ? 'answer.ans' : 'input.in'}`
      }
      const value = await api.putApiAuthoringProblemsIdWorkingCopyEntriesEntryId(id, entryId, {
        etag: copy.etag,
        entry: { id: entryId, path, kind, attributes },
        text,
      })
      saved(value)
      setSelected(entryId)
      setCreating(false)
    } catch (error) {
      setCreateError(apiError(error, '添加失败'))
    } finally {
      setOperation(null)
    }
  }
  async function upload(files: File[]) {
    if (!copy) return
    setOperation('upload')
    setUploadNotice('')
    setUploadProgress({ done: 0, total: files.length })
    setError('')
    try {
      if (!files.length || files.length > 200)
        throw new FormValidationError('一次选择 1–200 份文件；更多数据可用题包导入。')
      const paths = new Set(copy.tree.entries.map((entry) => entry.path.toLowerCase()))
      const pending = files.map((file) => {
        const kind =
          section === 'programs'
            ? 'source'
            : section === 'tests'
              ? file.name.toLowerCase().endsWith('.ans') || file.name.toLowerCase().endsWith('.out')
                ? 'answer'
                : 'input'
              : 'asset'
        const path = `${kind === 'source' ? 'sources' : kind === 'asset' ? 'attachments' : 'data'}/${file.webkitRelativePath || file.name}`
        if (paths.has(path.toLowerCase()))
          throw new FormValidationError(`文件路径重复：${path}。可先重命名，或在现有文件中编辑。`)
        paths.add(path.toLowerCase())
        return { file, path, kind, id: crypto.randomUUID() }
      })
      const entries = [...copy.tree.entries]
      for (let offset = 0; offset < pending.length; offset += 4) {
        const added = await Promise.all(
          pending.slice(offset, offset + 4).map(async (item): Promise<DomainTreeEntry> => ({
            id: item.id,
            path: item.path,
            kind: item.kind,
            attributes:
              item.kind === 'source'
                ? {
                    language: item.file.name.toLowerCase().endsWith('.py')
                      ? 'python'
                      : item.file.name.toLowerCase().endsWith('.c')
                        ? 'c'
                        : 'cpp',
                  }
                : {},
            blob: await api.postApiAuthoringProblemsIdBlobs(id, { file: item.file }),
          })),
        )
        entries.push(...added)
        setUploadProgress({ done: Math.min(offset + 4, pending.length), total: pending.length })
      }
      const value = await api.putApiAuthoringProblemsIdWorkingCopy(id, {
        etag: copy.etag,
        tree: { entries },
      })
      saved(value)
      setUploadNotice(
        `已保存 ${pending.length} 份文件。${section === 'programs' ? '在程序配置中关联源文件后即可检查。' : section === 'tests' ? '可在“数据文件”中查看，并关联到测试点。' : ''}`,
      )
      if (pending.length === 1 || section === 'programs')
        setSelected(
          (pending.find((item) => /^main\.(cpp|cc|cxx|c|py)$/i.test(item.file.name)) ?? pending[0])
            .id,
        )
    } catch (error) {
      setError(apiError(error, '上传失败'))
    } finally {
      setOperation(null)
      if (fileInput.current) fileInput.current.value = ''
      if (folderInput.current) folderInput.current.value = ''
    }
  }
  const navigation = (
    <nav aria-label="出题工作区" className="flex flex-col gap-1">
      {sections.map((item, index) => (
        <Button
          key={item.id}
          variant={section === item.id ? 'secondary' : 'ghost'}
          className={`justify-start ${index === 5 ? 'mt-4 border-t pt-2' : ''}`}
          disabled={busy || editorBusy}
          onClick={() => void chooseSection(item.id)}
          aria-current={section === item.id ? 'page' : undefined}
        >
          <item.icon className="size-4" />
          {item.label}
          {item.id === 'changes' && copy?.mergeId && (
            <span className="ml-auto size-2 rounded-full bg-amber-500" />
          )}
        </Button>
      ))}
    </nav>
  )
  if (loading) return <PageSpinner />
  if (!problem)
    return (
      <EmptyState
        title="无法打开出题工作台"
        description={error}
        action={<Button onClick={() => void load()}>重试</Button>}
      />
    )
  if (!current)
    return (
      <EmptyState
        title="页面不存在"
        action={<Button onClick={() => void chooseSection('statement')}>返回题面</Button>}
      />
    )
  const entries = copy?.tree.entries.filter((entry) => current.kinds.includes(entry.kind)) ?? []
  const entry =
    entries.find((entry) => entry.id === selected) ??
    (['tests', 'assets'].includes(section)
      ? undefined
      : section === 'programs'
        ? (entries.find((entry) => entry.kind === 'program') ?? entries[0])
        : entries[0])
  const editor =
    entry && copy ? (
      <MaterialEditor
        key={entry.id}
        problemId={id}
        entry={entry}
        copy={copy}
        canEdit={canEdit && !copy.mergeId}
        onSaved={saved}
        onDirty={setDirty}
        onSaving={setEditorBusy}
      />
    ) : (
      <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
        选择材料开始编辑，或添加新的材料。
      </div>
    )
  async function closeMaterial() {
    if (editorBusy || busy) return
    if (
      dirty &&
      !(await confirm({
        title: '放弃尚未保存的输入？',
        description: '已保存的副本与提交不受影响。',
        confirmLabel: '放弃本次输入',
        destructive: true,
      }))
    )
      return
    setDirty(false)
    setSelected('')
  }
  return (
    <div className="site-container py-5 sm:py-6">
      <header className="mb-6 flex flex-wrap items-start justify-between gap-x-4 gap-y-3 border-b pb-5">
        <div className="flex min-w-0 basis-full items-start gap-2 sm:basis-0 sm:grow">
          <Dialog open={drawer} onOpenChange={setDrawer}>
            <DialogTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="shrink-0 lg:hidden"
                aria-label="打开出题菜单"
              >
                <Menu />
              </Button>
            </DialogTrigger>
            <DialogContent
              side="left"
              onOpenAutoFocus={(event) => {
                event.preventDefault()
                drawerTitle.current?.focus()
              }}
            >
              <DialogTitle className="sr-only">出题菜单</DialogTitle>
              <DialogDescription className="sr-only">
                切换题面、程序、测试数据和版本历史。
              </DialogDescription>
              <p
                ref={drawerTitle}
                tabIndex={-1}
                className="truncate pr-12 text-lg font-semibold outline-none"
              >
                {title}
              </p>
              {navigation}
            </DialogContent>
          </Dialog>
          <div className="min-w-0">
            <Link
              className="text-xs text-muted-foreground hover:text-foreground"
              to="/workspace/problems"
            >
              工作台 / 题目 #{id}
            </Link>
            <h1 className="mt-1 truncate text-xl font-semibold tracking-tight" title={title}>
              {title}
            </h1>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {canEdit ? '我的工作副本' : '审阅共享提交'}
              {copy?.baseRevision ? ` · 基于 r${copy.baseRevision}` : ' · 尚未提交'}
              {dirty ? ' · 有未保存输入' : ''}
            </p>
          </div>
        </div>
        <div className="flex w-full shrink-0 flex-wrap gap-2 sm:w-auto [&>*]:flex-1 sm:[&>*]:flex-none">
          {problem.publishedVersion > 0 && (
            <Button variant="outline" asChild>
              <Link to={`/problems/${id}`}>查看题目</Link>
            </Button>
          )}
          {canEdit && copy && (
            <>
              <Button
                variant="outline"
                loading={operation === 'update'}
                disabled={busy || dirty || editorBusy || Boolean(copy.mergeId)}
                onClick={() => void update()}
              >
                <ArrowDownToLine />
                更新副本
              </Button>
              {section !== 'changes' && (
                <Button
                  disabled={dirty || busy || editorBusy}
                  onClick={() => void chooseSection('changes')}
                >
                  <GitCommitHorizontal />
                  {copy.mergeId ? '解决冲突' : '提交更改'}
                </Button>
              )}
            </>
          )}
        </div>
      </header>
      <div className="grid min-w-0 gap-6 lg:grid-cols-[180px_minmax(0,1fr)] xl:gap-8">
        <aside className="hidden lg:block">
          <div className="sticky top-[calc(var(--app-header-height)+1rem)]">
            {navigation}
            <p className="mt-6 px-3 text-xs leading-5 text-muted-foreground">
              保存到自己的工作副本；提交后与协作者共享。
            </p>
          </div>
        </aside>
        <section aria-label="当前出题内容" className="min-w-0">
          {error && (
            <p
              role="alert"
              className="mb-4 rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
            >
              {error}
            </p>
          )}
          {!copy ? (
            <EmptyState
              title="尚无可审阅的提交"
              description="协作者提交更改后，内容会出现在这里。"
            />
          ) : section === 'changes' || section === 'history' ? (
            <ChangesPanel
              problemId={id}
              copy={copy}
              canEdit={canEdit}
              history={section === 'history'}
              onSaved={saved}
              onDirty={setDirty}
              onBusy={setEditorBusy}
            />
          ) : section === 'packages' ? (
            <PackagesPanel
              problemId={id}
              copy={copy}
              canEdit={canEdit}
              revision={revision}
              onSaved={saved}
              onBusy={setEditorBusy}
            />
          ) : section === 'checks' ? (
            <ChecksPanel
              problemId={id}
              copy={copy}
              canEdit={canEdit}
              revision={revision}
              onBusy={setEditorBusy}
            />
          ) : section === 'releases' ? (
            <ReleasesPanel
              problemId={id}
              canPublish={problem.permissions.publish}
              onBusy={setEditorBusy}
            />
          ) : section === 'collaboration' ? (
            <div className="space-y-5">
              <h2 className="text-lg font-semibold">协作与权限</h2>
              <ResourceCollaboration
                kind="problem"
                id={id}
                ownerId={problem.ownerId}
                ownerName={problem.ownerName}
                manage={problem.permissions.manageAccess}
                transfer={problem.permissions.transfer}
                onChanged={() => {
                  void api
                    .getApiAdminProblemsId(id)
                    .then(setProblem)
                    .catch((error) => setError(apiError(error, '权限已变化，请重新打开题目')))
                }}
              />
              <ResourcePolicyPanel
                problem={problem}
                onChanged={setProblem}
                onBusy={setEditorBusy}
                onDirty={setDirty}
              />
            </div>
          ) : (
            <div className="space-y-5">
              {operation === 'upload' && (
                <div role="status" className="space-y-2 text-sm text-muted-foreground">
                  <p>
                    正在上传材料 · {uploadProgress.done} / {uploadProgress.total}
                  </p>
                  <progress
                    aria-label="材料上传进度"
                    className="h-1.5 w-full accent-primary"
                    value={uploadProgress.done}
                    max={Math.max(1, uploadProgress.total)}
                  />
                </div>
              )}
              {uploadNotice && (
                <p role="status" className="text-sm text-muted-foreground">
                  {uploadNotice}
                </p>
              )}
              {section !== 'overview' && (
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex min-w-0 flex-wrap gap-2">
                    {section === 'statement' &&
                      entries.map((item) => (
                        <Button
                          key={item.id}
                          variant={entry?.id === item.id ? 'secondary' : 'outline'}
                          disabled={editorBusy || busy}
                          onClick={() => void chooseEntry(item)}
                          className="max-w-56"
                        >
                          <span className="truncate">{entryLabel(item)}</span>
                        </Button>
                      ))}
                    {(section !== 'statement' || !entries.length) && (
                      <h2 className="text-lg font-semibold">{current.label}</h2>
                    )}
                  </div>
                  {canEdit && (
                    <div className="flex gap-2">
                      {['programs', 'tests', 'assets'].includes(section) && (
                        <>
                          <input
                            ref={fileInput}
                            className="hidden"
                            type="file"
                            multiple
                            aria-label="上传材料"
                            onChange={(e) => {
                              const files = [...(e.target.files ?? [])]
                              if (files.length) void upload(files)
                            }}
                          />
                          <Button
                            variant="outline"
                            disabled={dirty || busy || editorBusy}
                            onClick={() => fileInput.current?.click()}
                          >
                            <ArrowUpFromLine />
                            上传文件
                          </Button>
                          {section === 'programs' && (
                            <>
                              <input
                                ref={folderInput}
                                className="hidden"
                                type="file"
                                multiple
                                {...{ webkitdirectory: '' }}
                                aria-label="上传程序目录"
                                onChange={(event) => {
                                  const files = [...(event.target.files ?? [])]
                                  if (files.length) void upload(files)
                                }}
                              />
                              <Button
                                variant="outline"
                                disabled={dirty || busy || editorBusy}
                                onClick={() => folderInput.current?.click()}
                              >
                                上传目录
                              </Button>
                            </>
                          )}
                        </>
                      )}
                      {section !== 'assets' && (
                        <Button
                          variant="outline"
                          disabled={dirty || busy || editorBusy}
                          onClick={startCreate}
                        >
                          <Plus />
                          添加
                        </Button>
                      )}
                    </div>
                  )}
                </div>
              )}
              {section === 'programs' || section === 'tests' || section === 'assets' ? (
                <MaterialBrowser
                  key={section}
                  problemId={id}
                  mode={section}
                  copy={copy}
                  revision={revision}
                  canEdit={canEdit && !copy.mergeId}
                  disabled={busy || editorBusy || dirty}
                  selected={entry?.id}
                  onSelect={(item) => void chooseEntry(item)}
                  onSaved={saved}
                  onBusy={setEditorBusy}
                >
                  {section === 'programs' ? <div inert={busy}>{editor}</div> : undefined}
                </MaterialBrowser>
              ) : (
                editor
              )}
              {['tests', 'assets'].includes(section) && (
                <Dialog
                  open={Boolean(entry)}
                  onOpenChange={(value) => {
                    if (!value) void closeMaterial()
                  }}
                >
                  <DialogContent
                    side="right"
                    className="w-[min(740px,100vw)]"
                    onOpenAutoFocus={(event) => {
                      event.preventDefault()
                      materialTitle.current?.focus()
                    }}
                  >
                    <DialogTitle ref={materialTitle} tabIndex={-1} className="pr-10 outline-none">
                      {entry ? entryLabel(entry) : '材料'}
                    </DialogTitle>
                    <DialogDescription>
                      {canEdit
                        ? '内容保存在你的工作副本中，提交后与协作者共享。'
                        : '正在审阅共享提交的材料。'}
                    </DialogDescription>
                    {editor}
                  </DialogContent>
                </Dialog>
              )}
            </div>
          )}
        </section>
      </div>
      <Dialog
        open={creating}
        onOpenChange={(value) => {
          if (!busy) setCreating(value)
        }}
      >
        <DialogContent>
          <DialogTitle>添加材料</DialogTitle>
          <DialogDescription>先保存到工作副本，之后可与其他更改一起提交。</DialogDescription>
          <form
            noValidate
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void create()
            }}
          >
            <Choice
              label="材料类型"
              value={kind}
              onChange={setKind}
              options={(current.kinds.length ? current.kinds : ['source'])
                .filter((kind) => kind !== 'metadata')
                .map((kind) => [kind, materialNames[kind]])}
            />
            {kind === 'statement' ? (
              <Field label="语言标识">
                <Input
                  aria-label="语言标识"
                  value={language}
                  onChange={(e) => setLanguage(e.target.value)}
                  placeholder="zh / en"
                />
              </Field>
            ) : (
              <Field label="名称">
                <Input
                  aria-label="材料名称"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={materialNames[kind]}
                />
              </Field>
            )}
            <Field label="文件路径" hint="留空使用默认路径。重命名不会改变材料的稳定标识。">
              <Input
                aria-label="新材料路径"
                value={filePath}
                onChange={(e) => setFilePath(e.target.value)}
                placeholder="例如 sources/solution.cpp"
              />
            </Field>
            {createError && (
              <p role="alert" className="text-sm text-destructive">
                {createError}
              </p>
            )}
            <div className="flex justify-end">
              <Button type="submit" loading={busy}>
                添加到副本
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
