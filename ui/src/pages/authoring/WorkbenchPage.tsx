import { standardStatement } from '@/lib/statement-template'
import { availableLocation } from './material-locations'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
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
  Package,
  CheckCheck,
  Upload,
  ChevronRight,
  CircleAlert,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
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
import DataPanel from './DataPanel'
import AssetsPanel from './AssetsPanel'
import TestDetail from './TestDetail'
import ChangesPanel from './ChangesPanel'
import PackagesPanel from './PackagesPanel'
import ChecksPanel from './ChecksPanel'
import ReleasesPanel from './ReleasesPanel'
import ProgramWorkspace from './ProgramWorkspace'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import { ChevronDown, Check, Languages } from 'lucide-react'
import ResourcePolicyPanel from './ResourcePolicyPanel'
import { Choice, Field } from './MaterialForm'
import MaterialFinder from './MaterialFinder'
import { materialDestination } from './material-search'

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
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedEntry = searchParams.get('entry') ?? ''
  const openedLocation = useRef('')
  const [locationError, setLocationError] = useState('')
  const [finderOpen, setFinderOpen] = useState(false)
  const [finding, setFinding] = useState(false)
  const [entryRequest, setEntryRequest] = useState(0)
  const [compactNav, setCompactNav] = useState(() => {
    try {
      return localStorage.getItem('vertex-authoring-compact-nav') === 'true'
    } catch {
      return false
    }
  })
  useEffect(() => {
    try {
      localStorage.setItem('vertex-authoring-compact-nav', String(compactNav))
    } catch {
      /* Optional preference. */
    }
  }, [compactNav])
  const [problem, setProblem] = useState<DtoProblemResponse>(),
    [copy, setCopy] = useState<DomainWorkingCopy>(),
    [title, setTitle] = useState(''),
    [defaultStatementLanguage, setDefaultStatementLanguage] = useState('zh'),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(true),
    [operation, setOperation] = useState<'update' | 'create' | null>(null)
  const busy = operation !== null
  const [dirty, setDirty] = useState(false),
    [editorBusy, setEditorBusy] = useState(false),
    [selected, setSelected] = useState(''),
    [drawer, setDrawer] = useState(false),
    [creating, setCreating] = useState(false),
    [kind, setKind] = useState('source'),
    [name, setName] = useState(''),
    [language, setLanguage] = useState('zh'),
    [sourceLanguage, setSourceLanguage] = useState('cpp'),
    [createError, setCreateError] = useState('')
  const [advancedTest, setAdvancedTest] = useState(false),
    [selectedLabel, setSelectedLabel] = useState(''),
    [changeCount, setChangeCount] = useState<number>()
  const saveBeforeLeave = useRef<(() => Promise<boolean>) | null>(null)
  const [revision, setRevision] = useState<number>(),
    generation = useRef(0),
    materialTitle = useRef<HTMLHeadingElement>(null),
    drawerTitle = useRef<HTMLParagraphElement>(null)
  const current = sections.find((item) => item.id === section)
  const canEdit = Boolean(problem?.permissions.edit)
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if (
        event.key.toLowerCase() !== 'k' ||
        !(event.metaKey || event.ctrlKey) ||
        event.altKey ||
        event.isComposing ||
        event.defaultPrevented
      )
        return
      if (document.querySelector('[role="dialog"], [role="alertdialog"]') && !finderOpen) return
      event.preventDefault()
      if (!busy && !editorBusy && !finding && copy) setFinderOpen((value) => !value)
    }
    window.addEventListener('keydown', shortcut)
    return () => window.removeEventListener('keydown', shortcut)
  }, [busy, editorBusy, finding, finderOpen, copy])
  useEffect(() => {
    if (!requestedEntry) {
      openedLocation.current = ''
      setLocationError('')
      return
    }
    if (!copy) return
    const location = `${section}:${requestedEntry}`
    if (openedLocation.current === location) return
    const entry = copy.tree.entries.find(
      (item) =>
        item.id === requestedEntry &&
        (current?.kinds.includes(item.kind) || (section === 'tests' && item.kind === 'generation')),
    )
    openedLocation.current = location
    if (!entry) {
      setLocationError('这份材料已不在当前版本中，请从当前列表选择要查看的内容。')
      return
    }
    setLocationError('')
    if (entry.kind === 'generation') {
      setSelected('')
      return
    }
    setSelected(entry.id)
    setSelectedLabel(entry.attributes.label || entryLabel(entry))
    setAdvancedTest(false)
  }, [requestedEntry, section, copy, current])
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
  useEffect(() => {
    if (!copy || !canEdit) return
    let live = true
    setChangeCount(undefined)
    api
      .getApiAuthoringProblemsIdChanges(id)
      .then((result) => {
        if (live && (!result.etag || result.etag === copy.etag))
          setChangeCount(result.changes.length)
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [api, id, copy?.etag, canEdit])
  const metadataHash = copy?.tree.entries.find((entry) => entry.id === 'problem')?.blob.sha256
  useEffect(() => {
    if (!metadataHash) return
    let active = true
    api
      .getApiAuthoringProblemsIdMaterialsEntryId(id, 'problem', revision ? { revision } : undefined)
      .then((value) => {
        if (active && value.metadata) {
          setTitle(value.metadata.title)
          setDefaultStatementLanguage(value.metadata.statementLanguage)
        }
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
  async function chooseSection(value: string, reopen = false) {
    if (editorBusy || busy) return
    if (value === section && !reopen) {
      setDrawer(false)
      return
    }
    if (saveBeforeLeave.current) {
      if (!(await saveBeforeLeave.current())) return
    }
    if (
      dirty &&
      !saveBeforeLeave.current &&
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
    setError('')
    navigate(`/authoring/${id}/${value}`)
  }
  async function openFoundMaterial(entry: DomainTreeEntry, directSource = false) {
    if (busy || editorBusy || finding) return false
    const destination = materialDestination(id, entry)
    if (!destination) return false
    setFinding(true)
    try {
      if (saveBeforeLeave.current) {
        if (!(await saveBeforeLeave.current())) return false
      } else if (
        dirty &&
        !(await confirm({
          title: '打开其他材料前放弃未保存的输入？',
          description: '已保存的工作副本和历史提交不受影响。',
          confirmLabel: '放弃输入并打开',
          destructive: true,
        }))
      )
        return false
      setDirty(false)
      setAdvancedTest(false)
      setSelected(entry.id)
      setSelectedLabel(entry.attributes.label || entryLabel(entry))
      setDrawer(false)
      setEntryRequest((value) => value + 1)
      navigate(
        directSource && entry.kind === 'source' ? `${destination.to}&view=source` : destination.to,
        { replace: destination.section === section },
      )
      return true
    } finally {
      setFinding(false)
    }
  }
  async function chooseEntry(entry: DomainTreeEntry) {
    if (editorBusy || busy) return
    if (saveBeforeLeave.current) {
      if (!(await saveBeforeLeave.current())) return
    }
    if (
      dirty &&
      !saveBeforeLeave.current &&
      !(await confirm({
        title: '放弃当前未保存的输入？',
        description: '这次输入尚未保存，已保存内容不受影响。',
        confirmLabel: '切换材料',
        destructive: true,
      }))
    )
      return
    setDirty(false)
    setAdvancedTest(false)
    setSelectedLabel(entry.attributes.label || entryLabel(entry))
    setSelected(entry.id)
    setSearchParams(
      (params) => {
        params.set('entry', entry.id)
        return params
      },
      { replace: true },
    )
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
  async function startCreate() {
    if (editorBusy || busy || copy?.mergeId) return
    if (saveBeforeLeave.current && !(await saveBeforeLeave.current())) return
    setError('')
    setKind(current?.kinds.find((kind) => kind !== 'metadata') || 'source')
    setName('')
    setSourceLanguage('cpp')
    setLanguage(
      copy?.tree.entries.some(
        (entry) => entry.kind === 'statement' && entry.attributes.language === 'zh',
      )
        ? 'en'
        : 'zh',
    )
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
        path = '',
        attributes: Record<string, string> = {}
      if (kind === 'statement') {
        if (!/^[a-z]{2}(-[A-Za-z]{2,8})?$/.test(language))
          throw new FormValidationError('语言标识应为 zh、en 等格式')
        if (
          copy.tree.entries.some(
            (entry) => entry.kind === 'statement' && entry.attributes.language === language,
          )
        )
          throw new FormValidationError('这个语言已有题面，请直接编辑已有内容。')
        path = `statement/problem.${language}.md`
        attributes = { format: 'markdown', language }
        text = standardStatement(title, language)
      } else if (kind === 'program') {
        path = `vertex/programs/program-${copy.tree.entries.filter((e) => e.kind === 'program').length + 1}.json`
        text = JSON.stringify({ ...defaultProgram(), name: label })
        attributes = { format: 'json', label }
      } else if (kind === 'test') {
        path = `vertex/tests/test-${copy.tree.entries.filter((e) => e.kind === 'test').length + 1}.json`
        text = JSON.stringify({ ...defaultTest(), name: label })
        attributes = { format: 'json', label }
      } else if (kind === 'validation') {
        path = `vertex/validation/case-${copy.tree.entries.filter((e) => e.kind === 'validation').length + 1}.json`
        text = JSON.stringify({
          schemaVersion: 1,
          name: label,
          mode: 'invalid_output',
          input: '',
          answer: '',
          output: '',
          description: '',
        })
        attributes = { format: 'json', label }
      } else if (kind === 'group') {
        path = `vertex/groups/group-${copy.tree.entries.filter((e) => e.kind === 'group').length + 1}.json`
        text = JSON.stringify({ ...defaultGroup(), name: label })
        attributes = { format: 'json', label }
      } else if (kind === 'source') {
        text =
          '#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n    return 0;\n}\n'
        path = `sources/${entryId}/${sourceLanguage === 'python' ? 'main.py' : sourceLanguage === 'c' ? 'main.c' : 'main.cpp'}`
        text =
          sourceLanguage === 'python'
            ? 'import sys\n\ndef main():\n    pass\n\nif __name__ == "__main__":\n    main()\n'
            : sourceLanguage === 'c'
              ? '#include <stdio.h>\n\nint main(void) {\n    return 0;\n}\n'
              : text
        attributes = {
          language: sourceLanguage,
          format: 'text',
          label: name.trim() || path.split('/').pop()!,
        }
      } else {
        path = `data/${kind === 'answer' ? 'answer.ans' : 'input.in'}`
        attributes = { label }
      }
      const value = await api.putApiAuthoringProblemsIdWorkingCopyEntriesEntryId(id, entryId, {
        etag: copy.etag,
        entry: {
          id: entryId,
          path: availableLocation(path, copy.tree.entries, entryId),
          kind,
          attributes,
        },
        text,
      })
      saved(value)
      setSelectedLabel(label)
      setSelected(entryId)
      setCreating(false)
    } catch (error) {
      setCreateError(apiError(error, '添加失败'))
    } finally {
      setOperation(null)
    }
  }
  const navigation = (compact = false) => (
    <nav aria-label="出题工作区" className="space-y-6">
      {[
        { label: '编写题目', ids: ['statement', 'programs', 'tests', 'assets'] },
        { label: '版本与交付', ids: ['changes', 'checks'] },
        { label: '工具与设置', ids: ['overview', 'packages', 'collaboration'] },
      ].map((group) => (
        <div key={group.label}>
          <p
            className={
              compact
                ? 'sr-only'
                : 'mb-2 px-3 text-[11px] font-medium tracking-wide text-muted-foreground/75'
            }
          >
            {group.label}
          </p>
          <div className="space-y-1">
            {group.ids.map((id) => {
              const item = sections.find((s) => s.id === id)!,
                active =
                  section === id ||
                  (id === 'changes' && section === 'history') ||
                  (id === 'checks' && section === 'releases')
              return (
                <button
                  key={id}
                  disabled={busy || editorBusy}
                  onClick={() => void chooseSection(id)}
                  aria-current={active ? 'page' : undefined}
                  title={compact ? item.label : undefined}
                  aria-label={compact ? item.label : undefined}
                  className={`flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition-colors ${active ? 'bg-primary/8 font-medium text-primary' : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'}`}
                >
                  <item.icon className="size-4 shrink-0" />
                  <span className={compact ? 'sr-only' : undefined}>
                    {(
                      {
                        changes: '版本管理',
                        checks: '检查与发布',
                        assets: '图片与附件',
                        packages: '导入 / 导出',
                      } as Record<string, string>
                    )[id] ?? item.label}
                  </span>
                  {!compact && id === 'changes' && !!changeCount && (
                    <span className="ml-auto rounded bg-primary/10 px-1.5 text-xs tabular-nums">
                      {changeCount}
                    </span>
                  )}
                  {!compact && ['statement', 'programs', 'tests', 'assets'].includes(id) && (
                    <span className="ml-auto text-xs tabular-nums text-muted-foreground/70">
                      {copy?.tree.entries.filter(
                        (entry) =>
                          (id === 'assets' && entry.kind === 'resource') ||
                          entry.kind ===
                            (
                              {
                                statement: 'statement',
                                programs: 'program',
                                tests: 'test',
                                assets: 'asset',
                              } as Record<string, string>
                            )[id],
                      ).length ?? 0}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>
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
  const directSource =
    section === 'programs' && searchParams.get('view') === 'source'
      ? entries.find((entry) => entry.id === requestedEntry && entry.kind === 'source')
      : undefined
  const entry =
    directSource ??
    entries.find((entry) => entry.id === selected) ??
    (['tests', 'assets'].includes(section)
      ? undefined
      : section === 'programs'
        ? (entries.find((entry) => entry.kind === 'program') ?? entries[0])
        : section === 'statement'
          ? (entries.find((entry) => entry.attributes.language === defaultStatementLanguage) ??
            entries[0])
          : entries[0])
  const editor =
    entry && copy ? (
      entry.kind === 'test' && !advancedTest ? (
        <TestDetail
          problemId={id}
          entry={entry}
          copy={copy}
          canEdit={canEdit && !copy.mergeId}
          onSaved={saved}
          onDirty={setDirty}
          onBusy={setEditorBusy}
          onOpenGeneration={() => {
            setSelected('')
            navigate(
              `/authoring/${id}/tests?data=generation&plan=${encodeURIComponent(entry.attributes.generationPlan)}`,
            )
          }}
          onAdvanced={() => setAdvancedTest(true)}
        />
      ) : (
        <MaterialEditor
          statementNavigation={
            section === 'statement' ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy || editorBusy}
                    className="gap-2"
                    aria-label="切换题面语言"
                  >
                    <Languages className="size-4" />
                    {entryLabel(entry)}
                    <ChevronDown className="size-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  {entries.map((item) => (
                    <DropdownMenuItem key={item.id} onSelect={() => void chooseEntry(item)}>
                      <span className="flex-1">{entryLabel(item)}</span>
                      {item.id === entry.id && <Check className="size-4" />}
                    </DropdownMenuItem>
                  ))}
                  {canEdit && (
                    <DropdownMenuItem
                      disabled={Boolean(copy.mergeId)}
                      onSelect={() => void startCreate()}
                    >
                      <Plus className="size-4" />
                      添加其他语言
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : undefined
          }
          beforeLeave={saveBeforeLeave}
          key={entry.id}
          problemId={id}
          entry={entry}
          copy={copy}
          canEdit={canEdit && !copy.mergeId}
          onSaved={saved}
          onDirty={setDirty}
          onSaving={setEditorBusy}
        />
      )
    ) : (
      <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
        {section === 'statement' ? (
          <div className="space-y-4">
            <p>先添加一份题面，开始描述题目。</p>
            {canEdit && (
              <Button disabled={Boolean(copy?.mergeId)} onClick={() => void startCreate()}>
                添加题面
              </Button>
            )}
          </div>
        ) : (
          '选择材料开始编辑，或添加新的材料。'
        )}
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
    setSearchParams(
      (params) => {
        params.delete('entry')
        return params
      },
      { replace: true },
    )
  }
  return (
    <div className="authoring-workbench site-container max-w-[1600px] py-5 sm:py-6">
      <header className="mb-5 flex flex-wrap items-start justify-between gap-x-4 gap-y-4">
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
              {navigation()}
            </DialogContent>
          </Dialog>
          <Button
            variant="ghost"
            size="icon"
            className="hidden shrink-0 lg:inline-flex"
            aria-label={compactNav ? '展开工作区导航' : '收起工作区导航'}
            title={compactNav ? '展开工作区导航' : '收起导航，扩大编辑区域'}
            aria-expanded={!compactNav}
            onClick={() => setCompactNav((value) => !value)}
          >
            {compactNav ? <PanelLeftOpen /> : <PanelLeftClose />}
          </Button>
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
            <p className="mt-1 flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground">
              <span role="status" className={dirty ? 'text-verdict-tle' : undefined}>
                {canEdit
                  ? dirty
                    ? editorBusy
                      ? '正在保存编辑…'
                      : '有未保存的编辑'
                    : '个人草稿已保存'
                  : '审阅已提交版本'}
              </span>
              {copy?.baseRevision ? ` · 基于 r${copy.baseRevision}` : ' · 尚未提交'}
              {!!changeCount ? ` · ${changeCount} 项待提交` : ''}
            </p>
          </div>
        </div>
        <div className="flex w-full shrink-0 flex-wrap gap-2 sm:w-auto [&>*]:flex-1 sm:[&>*]:flex-none">
          <Button
            variant="outline"
            disabled={!copy || busy || editorBusy || finding}
            onClick={() => setFinderOpen(true)}
            title="查找题目材料 · Ctrl / ⌘ K"
          >
            <Search />
            查找材料
            <kbd className="ml-2 hidden rounded border px-1.5 py-0.5 text-[10px] text-muted-foreground lg:inline">
              ⌘ / Ctrl K
            </kbd>
          </Button>
          {problem.publishedVersion > 0 && (
            <Button variant="outline" asChild>
              <Link to={`/problems/${id}`}>查看发布版</Link>
            </Button>
          )}
          {canEdit && copy && (
            <>
              {
                <Button
                  variant="outline"
                  size={
                    Number(copy.headRevision ?? 0) > Number(copy.baseRevision ?? 0)
                      ? 'default'
                      : 'icon'
                  }
                  aria-label="同步协作者更新"
                  style={
                    Number(copy.headRevision ?? 0) > Number(copy.baseRevision ?? 0)
                      ? undefined
                      : { flex: '0 0 auto' }
                  }
                  title="检查并同步协作者的更新"
                  loading={operation === 'update'}
                  disabled={busy || dirty || editorBusy || Boolean(copy.mergeId)}
                  onClick={() => void update()}
                >
                  <ArrowDownToLine />
                  {Number(copy.headRevision ?? 0) > Number(copy.baseRevision ?? 0) &&
                    '同步协作者更新'}
                </Button>
              }
              {section !== 'changes' && (
                <Button disabled={busy || editorBusy} onClick={() => void chooseSection('changes')}>
                  <GitCommitHorizontal />
                  {copy.mergeId ? '解决冲突' : '审阅更改'}
                </Button>
              )}
            </>
          )}
        </div>
        <nav aria-label="出题流程" className="w-full rounded-xl border bg-card">
          <ol className="grid grid-cols-4">
            {[
              {
                id: 'statement',
                label: '编写',
                detail: '题面、程序与数据',
                active: !['changes', 'history', 'checks', 'releases'].includes(section),
              },
              {
                id: 'changes',
                label: '提交',
                detail: changeCount ? `${changeCount} 项待审阅` : '与协作者共享',
                active: ['changes', 'history'].includes(section),
              },
              {
                id: 'checks',
                label: '检查',
                detail: '运行并核对结果',
                active: section === 'checks',
              },
              {
                id: 'releases',
                label: '发布',
                detail: problem.publishedVersion
                  ? `当前 v${problem.publishedVersion}`
                  : '交付给选手',
                active: section === 'releases',
              },
            ].map((step, index) => (
              <li key={step.id} className="relative min-w-0">
                <button
                  type="button"
                  aria-current={step.active ? 'step' : undefined}
                  disabled={busy || editorBusy}
                  onClick={() => void chooseSection(step.id)}
                  className={`flex w-full items-center justify-center gap-1.5 rounded-lg px-1 py-3 text-left transition-colors hover:bg-accent disabled:opacity-50 sm:justify-start sm:gap-2 sm:px-4 ${step.active ? 'bg-primary/5 text-primary' : 'text-muted-foreground'}`}
                >
                  <span
                    className={`flex size-5 shrink-0 items-center justify-center rounded-full text-xs tabular-nums sm:size-6 ${step.active ? 'bg-primary text-primary-foreground' : 'bg-muted'}`}
                  >
                    {index + 1}
                  </span>
                  <span className="min-w-0">
                    <span className="block whitespace-nowrap text-xs font-medium sm:text-sm">
                      {step.label}
                    </span>
                    <span className="mt-0.5 hidden truncate text-xs text-muted-foreground sm:block">
                      {step.detail}
                    </span>
                  </span>
                </button>
                {index < 3 && (
                  <ChevronRight
                    aria-hidden="true"
                    className="pointer-events-none absolute -right-1.5 top-1/2 z-10 hidden size-3 -translate-y-1/2 text-muted-foreground/50 sm:block"
                  />
                )}
              </li>
            ))}
          </ol>
        </nav>
      </header>
      <div
        className={`grid min-w-0 gap-5 ${compactNav ? 'lg:grid-cols-[44px_minmax(0,1fr)]' : 'lg:grid-cols-[180px_minmax(0,1fr)]'}`}
      >
        <aside className="hidden lg:block">
          <div className="sticky top-[calc(var(--app-header-height)+1rem)]">
            {navigation(compactNav)}
          </div>
        </aside>
        <section aria-label="当前出题内容" className="min-w-0">
          {locationError && (
            <p
              role="status"
              className="mb-4 rounded-lg border bg-muted/30 p-3 text-sm text-muted-foreground"
            >
              {locationError}
            </p>
          )}
          {copy?.mergeId && section !== 'changes' && (
            <div
              role="status"
              className="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-verdict-tle/25 bg-verdict-tle-bg p-3 text-sm"
            >
              <CircleAlert className="size-4 shrink-0 text-verdict-tle" />
              <p className="min-w-0 flex-1">副本有待解决的冲突，处理后可继续编辑。</p>
              <Button
                size="sm"
                variant="outline"
                disabled={busy || editorBusy}
                onClick={() => void chooseSection('changes')}
              >
                解决冲突
              </Button>
            </div>
          )}
          {error && (
            <p
              role="alert"
              className="mb-4 rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
            >
              {error}
            </p>
          )}
          {['changes', 'history', 'checks', 'releases'].includes(section) && (
            <div className="mb-6 flex gap-1 border-b pb-3">
              {(section === 'changes' || section === 'history'
                ? [
                    ['changes', '当前更改'],
                    ['history', '提交历史'],
                  ]
                : [
                    ['checks', '运行检查'],
                    ['releases', '发布版本'],
                  ]
              ).map(([id, label]) => (
                <Button
                  key={id}
                  size="sm"
                  variant={section === id ? 'secondary' : 'ghost'}
                  disabled={busy || editorBusy}
                  onClick={() => void chooseSection(id)}
                >
                  {label}
                </Button>
              ))}
            </div>
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
              canCheck={canEdit}
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
              {section === 'tests' ? (
                <DataPanel
                  key={entryRequest}
                  onDirty={setDirty}
                  problemId={id}
                  copy={copy}
                  revision={revision}
                  canEdit={canEdit && !copy.mergeId}
                  disabled={busy || editorBusy || dirty}
                  onSaved={saved}
                  onBusy={setEditorBusy}
                  onSelect={(item) => void chooseEntry(item)}
                  onAdvancedCreate={(kind) => {
                    setKind(kind)
                    setName('')
                    setSourceLanguage('cpp')
                    setCreating(true)
                  }}
                />
              ) : section === 'assets' ? (
                <AssetsPanel
                  initialEntryId={requestedEntry || undefined}
                  initialEntryRequest={entryRequest}
                  problemId={id}
                  copy={copy}
                  canEdit={canEdit && !copy.mergeId}
                  onSaved={saved}
                  onBusy={setEditorBusy}
                />
              ) : section === 'programs' ? (
                directSource ? (
                  <div className="space-y-4">
                    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/20 p-3">
                      <p className="min-w-0 break-all text-xs text-muted-foreground">
                        源文件 · {directSource.path}
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy || editorBusy || finding}
                        onClick={() => void chooseSection('programs', true)}
                      >
                        返回程序列表
                      </Button>
                    </div>
                    {editor}
                  </div>
                ) : (
                  <ProgramWorkspace
                    initialEntryId={requestedEntry || undefined}
                    initialEntryRequest={entryRequest}
                    problemId={id}
                    copy={copy}
                    revision={revision}
                    canEdit={canEdit && !copy.mergeId}
                    onSaved={saved}
                    onDirty={setDirty}
                    onBusy={setEditorBusy}
                    beforeLeave={saveBeforeLeave}
                    onOpenSource={(entry) => openFoundMaterial(entry, true)}
                  />
                )
              ) : (
                editor
              )}
              {section === 'tests' && (
                <Dialog
                  open={Boolean(entry)}
                  onOpenChange={(value) => {
                    if (!value) void closeMaterial()
                  }}
                >
                  <DialogContent
                    side="right"
                    className="w-[min(940px,100vw)]"
                    onOpenAutoFocus={(event) => {
                      event.preventDefault()
                      materialTitle.current?.focus()
                    }}
                  >
                    <DialogTitle ref={materialTitle} tabIndex={-1} className="pr-10 outline-none">
                      {entry ? selectedLabel || entryLabel(entry) : '测试点'}
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
      {copy && (
        <MaterialFinder
          open={finderOpen}
          onOpenChange={setFinderOpen}
          problemId={id}
          entries={copy.tree.entries}
          etag={copy.etag}
          revision={revision}
          onSelect={openFoundMaterial}
        />
      )}
      <Dialog
        open={creating}
        onOpenChange={(value) => {
          if (!busy) setCreating(value)
        }}
      >
        <DialogContent>
          <DialogTitle>{kind === 'statement' ? '添加题面语言' : '添加内容'}</DialogTitle>
          <DialogDescription>先保存到工作副本，之后可与其他更改一起提交。</DialogDescription>
          <form
            noValidate
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void create()
            }}
          >
            {current.kinds.length > 1 && (
              <Choice
                label="添加内容"
                value={kind}
                onChange={setKind}
                options={(current.kinds.length ? current.kinds : ['source'])
                  .filter((kind) => kind !== 'metadata')
                  .map((kind) => [kind, materialNames[kind]])}
              />
            )}
            {kind === 'statement' ? (
              <div className="space-y-3">
                <Choice
                  label="题面语言"
                  value={
                    ['zh', 'en', 'ja', 'ko', 'ru', 'fr', 'de', 'es'].includes(language)
                      ? language
                      : 'custom'
                  }
                  onChange={(value) => setLanguage(value === 'custom' ? '' : value)}
                  options={[
                    ['zh', '中文'],
                    ['en', 'English'],
                    ['ja', '日本語'],
                    ['ko', '한국어'],
                    ['ru', 'Русский'],
                    ['fr', 'Français'],
                    ['de', 'Deutsch'],
                    ['es', 'Español'],
                    ['custom', '其他语言'],
                  ]}
                />
                {!['zh', 'en', 'ja', 'ko', 'ru', 'fr', 'de', 'es'].includes(language) && (
                  <Input
                    aria-label="语言标识"
                    value={language}
                    onChange={(event) => setLanguage(event.target.value)}
                    placeholder="语言代码，例如 pt"
                  />
                )}
              </div>
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
            {kind === 'source' && (
              <Choice
                label="编程语言"
                value={sourceLanguage}
                onChange={setSourceLanguage}
                options={[
                  ['cpp', 'C++'],
                  ['c', 'C'],
                  ['python', 'Python'],
                ]}
              />
            )}
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
