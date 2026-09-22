import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  Bold,
  Italic,
  ChevronDown,
  Strikethrough,
  ListOrdered,
  Link2,
  Braces,
  List,
  Quote,
  ImagePlus,
  Sigma,
  Table2,
  Code2,
  Undo2,
  Redo2,
  Search,
  ListTree,
  Plus,
  Upload,
  File,
  X,
  MoreHorizontal,
} from 'lucide-react'
import type { DomainTreeEntry } from '@/generated/api/model'
import CodeEditor, { type EditorCommands } from '@/components/CodeEditor'
import SplitPane from '@/components/SplitPane'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import MdRenderer from '@/components/MdRenderer'
import { entryLabel } from '@/lib/authoring-materials'
import { apiError, formatFileSize } from '@/lib/format'
import { statementOutline } from '@/lib/statement-editing'
import { attachmentMarkdown } from './asset-links'
import StatementPreview from './StatementPreview'
import AttachmentPreview from './AttachmentPreview'
import { useStatementScrollSync } from './useStatementScrollSync'

export default function StatementComposer({
  etag,
  revision,
  navigation,
  actions,
  problemId,
  entry,
  entries,
  value,
  onChange,
  readOnly,
  fullscreen,
  focused = false,
  status,
  onUpload,
}: {
  etag: string
  revision?: number
  navigation?: ReactNode
  actions?: ReactNode
  problemId: string
  entry: DomainTreeEntry
  entries: DomainTreeEntry[]
  value: string
  onChange: (value: string) => void
  readOnly: boolean
  fullscreen: boolean
  focused?: boolean
  status: string
  onUpload: (file: File) => Promise<DomainTreeEntry>
}) {
  const commands = useRef<EditorCommands | null>(null),
    previewRef = useRef<HTMLElement | null>(null),
    composerRef = useRef<HTMLDivElement>(null),
    upload = useRef<HTMLInputElement>(null)
  const markdown = entry.attributes.format === 'markdown'
  const [preferredMode, setMode] = useState<'edit' | 'split' | 'preview'>(() =>
    window.matchMedia('(min-width:1024px)').matches ? 'split' : 'edit',
  )
  const [wide, setWide] = useState(false)
  useEffect(() => {
    const root = composerRef.current
    if (!root) return
    const observer = new ResizeObserver(([entry]) => setWide(entry.contentRect.width >= 760))
    observer.observe(root)
    return () => observer.disconnect()
  }, [])
  const mode = preferredMode === 'split' && !wide ? 'edit' : preferredMode
  const [scrollSync, setScrollSync] = useState(true)
  const [selection, setSelection] = useState({ from: 0, line: 1, column: 1, characters: 0 })
  useStatementScrollSync(
    commands,
    previewRef,
    markdown && mode === 'split' && scrollSync,
    entry.id + '\0' + value,
  )
  const [outline, setOutline] = useState(false),
    [dialog, setDialog] = useState<'assets' | 'formula' | 'link' | null>(null)
  const [linkLabel, setLinkLabel] = useState(''),
    [linkURL, setLinkURL] = useState('')
  const [query, setQuery] = useState(''),
    [selected, setSelected] = useState(''),
    [error, setError] = useState(''),
    [uploading, setUploading] = useState(false)
  const [formula, setFormula] = useState('a^2 + b^2 = c^2'),
    [blockMath, setBlockMath] = useState(true)
  const headings = useMemo(() => statementOutline(value, !markdown), [value, markdown])
  const currentHeading = [...headings].reverse().find((heading) => heading.from <= selection.from)
  const files = entries.filter(
    (e) =>
      e.kind === 'asset' &&
      e.attributes.visibility !== 'private' &&
      (markdown || /\.(png|jpe?g|pdf)$/i.test(e.path)),
  )
  const visible = files.filter((e) => entryLabel(e).toLowerCase().includes(query.toLowerCase()))
  const editing = !readOnly && !uploading && mode !== 'preview'
  function insert(content: string) {
    setDialog(null)
    requestAnimationFrame(() => commands.current?.replace(content))
  }
  function jumpToSection(from: number) {
    if (mode === 'preview') {
      const line = value.slice(0, from).split('\n').length - 1
      const target = previewRef.current?.querySelector<HTMLElement>(`[data-source-line="${line}"]`)
      if (target && previewRef.current) {
        previewRef.current.scrollTop +=
          target.getBoundingClientRect().top - previewRef.current.getBoundingClientRect().top - 24
      }
    } else commands.current?.jump(from)
    if (!wide) setOutline(false)
  }
  function insertAsset(file: DomainTreeEntry) {
    try {
      insert('\n\n' + attachmentMarkdown(entry, file) + '\n\n')
    } catch (e) {
      setError(apiError(e, '无法插入此附件'))
    }
  }
  async function add(file: File) {
    setUploading(true)
    setError('')
    try {
      const result = await onUpload(file)
      setSelected(result.id)
      setQuery('')
    } catch (e) {
      setError(apiError(e, '上传失败，请重试'))
    } finally {
      setUploading(false)
      if (upload.current) upload.current.value = ''
    }
  }
  return (
    <div
      ref={composerRef}
      className={`@container min-w-0 max-w-full overflow-hidden rounded-xl border bg-card ${focused ? 'flex min-h-[360px] flex-1 flex-col' : fullscreen ? 'flex h-[calc(100dvh-4rem)] flex-col' : ''}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          {navigation}
          <span className="text-xs text-muted-foreground">{markdown ? 'Markdown' : 'TeX'}</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {markdown ? (
            <div className="flex rounded-lg border bg-background p-0.5" aria-label="题面显示方式">
              {(['edit', ...(wide ? ['split' as const] : []), 'preview'] as const).map((item) => (
                <button
                  key={item}
                  type="button"
                  aria-pressed={mode === item}
                  onClick={() => setMode(item)}
                  className={`rounded-md px-3 py-1.5 text-xs transition-colors ${mode === item ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted'}`}
                >
                  {{ edit: '写作', split: '对照', preview: '预览' }[item]}
                </button>
              ))}
            </div>
          ) : (
            <span className="text-xs text-muted-foreground">TeX · 在检查页编译 PDF</span>
          )}
          {markdown && mode === 'split' && (
            <Button
              variant="ghost"
              size="icon"
              aria-label="同步滚动"
              aria-pressed={scrollSync}
              title={scrollSync ? '同步滚动已开启' : '开启同步滚动'}
              onClick={() => setScrollSync(!scrollSync)}
              className={scrollSync ? 'bg-primary/10 text-primary' : 'text-muted-foreground'}
            >
              <Link2 className="size-4" />
            </Button>
          )}
          {actions}
        </div>
      </div>
      <div
        className="flex flex-wrap items-center gap-1 border-b bg-muted/20 px-2 py-1.5 [&>*]:shrink-0"
        aria-label={mode === 'preview' || readOnly ? '题面阅读工具' : '题面格式工具'}
      >
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label="题面大纲"
            title="题面大纲"
            aria-pressed={outline}
            onClick={() => setOutline(!outline)}
          >
            <ListTree />
          </Button>
          {readOnly && mode !== 'preview' && (
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label="查找题面"
              title="查找题面"
              onClick={() => commands.current?.search()}
            >
              <Search />
            </Button>
          )}
        </div>
        {mode === 'preview' || readOnly ? (
          <span className="px-2 text-xs text-muted-foreground">
            {readOnly ? '只读题面' : '预览当前内容，切换到写作继续编辑'}
          </span>
        ) : (
          <>
            <div className="flex items-center gap-1">
              <span className="mr-1 h-5 border-l" />
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label="撤销编辑"
                title="撤销 · Ctrl / ⌘ Z"
                disabled={!editing}
                onClick={() => commands.current?.undo()}
              >
                <Undo2 />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label="重做编辑"
                title="重做 · Ctrl / ⌘ Shift Z"
                disabled={!editing}
                onClick={() => commands.current?.redo()}
              >
                <Redo2 />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="查找与替换"
                title="查找与替换 · Ctrl / ⌘ F"
                className="hidden size-8 @min-[760px]:inline-flex"
                onClick={() => commands.current?.search()}
              >
                <Search />
              </Button>
            </div>
            <span className="mx-1 hidden h-5 border-l @min-[760px]:block" />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="hidden @min-[760px]:inline-flex"
                  disabled={!editing}
                  aria-label="标题级别"
                >
                  标题级别
                  <ChevronDown className="size-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                portalContainer={
                  fullscreen ? (document.fullscreenElement as HTMLElement) : undefined
                }
              >
                {(markdown ? [1, 2, 3, 4, 5, 6] : [1, 2, 3]).map((level) => (
                  <DropdownMenuItem
                    key={level}
                    onSelect={() => commands.current?.format('heading', !markdown, level)}
                  >
                    <span className="w-6 font-mono text-xs text-muted-foreground">H{level}</span>
                    {level === 1 ? '题目标题' : level === 2 ? '章节标题' : `${level} 级标题`}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            {(
              [
                { id: 'bold', label: '加粗', icon: Bold },
                { id: 'italic', label: '斜体', icon: Italic },
                { id: 'strike', label: '删除线', icon: Strikethrough },
                { id: 'code', label: '行内代码', icon: Code2 },
                { id: 'list', label: '无序列表', icon: List },
                { id: 'ordered', label: '有序列表', icon: ListOrdered },
                { id: 'quote', label: '引用', icon: Quote },
              ] as const
            ).map(({ id, label, icon: Icon }) => (
              <Button
                key={id}
                variant="ghost"
                size="icon"
                className={`size-8 ${['bold', 'italic'].includes(id) ? '' : 'hidden @min-[760px]:inline-flex'}`}
                aria-label={label}
                title={label}
                disabled={!editing}
                onClick={() => commands.current?.format(id, !markdown)}
              >
                <Icon />
              </Button>
            ))}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 @min-[760px]:hidden"
                  disabled={!editing}
                  aria-label="更多格式"
                >
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                portalContainer={
                  fullscreen ? (document.fullscreenElement as HTMLElement) : undefined
                }
              >
                <DropdownMenuItem onSelect={() => commands.current?.search()}>
                  <Search />
                  查找与替换
                </DropdownMenuItem>
                {(markdown ? [1, 2, 3, 4, 5, 6] : [1, 2, 3]).map((level) => (
                  <DropdownMenuItem
                    key={level}
                    onSelect={() => commands.current?.format('heading', !markdown, level)}
                  >
                    H{level} ·{' '}
                    {level === 1 ? '题目标题' : level === 2 ? '章节标题' : `${level} 级标题`}
                  </DropdownMenuItem>
                ))}
                {(
                  [
                    { id: 'strike', label: '删除线', icon: Strikethrough },
                    { id: 'code', label: '行内代码', icon: Code2 },
                    { id: 'list', label: '无序列表', icon: List },
                    { id: 'ordered', label: '有序列表', icon: ListOrdered },
                    { id: 'quote', label: '引用', icon: Quote },
                  ] as const
                ).map(({ id, label, icon: Icon }) => (
                  <DropdownMenuItem
                    key={id}
                    onSelect={() => commands.current?.format(id, !markdown)}
                  >
                    <Icon />
                    {label}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            <span className="mx-1 hidden h-5 border-l @min-[760px]:block" />
            {markdown && (
              <Button
                variant="ghost"
                size="icon"
                className="hidden @min-[760px]:inline-flex"
                disabled={!editing}
                aria-label="插入链接"
                title="插入链接"
                onClick={() => {
                  setError('')
                  setDialog('link')
                }}
              >
                <Link2 />
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              className="hidden @min-[760px]:inline-flex"
              disabled={!editing}
              onClick={() => {
                const marker = markdown ? '{{remainingsamples}}' : '\\remainingsamples'
                const position = value.indexOf(marker)
                if (position >= 0) commands.current?.jump(position)
                else insert('\n\n' + marker + '\n\n')
              }}
            >
              <Braces />
              样例
            </Button>
            {markdown && (
              <Button
                variant="ghost"
                size="sm"
                className="hidden @min-[760px]:inline-flex"
                disabled={!editing}
                onClick={() => insert('\n\n| 项目 | 说明 |\n| --- | --- |\n| 内容 | 内容 |\n\n')}
              >
                <Table2 />
                表格
              </Button>
            )}

            <Button
              variant="ghost"
              size="sm"
              className="hidden @min-[760px]:inline-flex"
              disabled={!editing}
              onClick={() => {
                setError('')
                setDialog('formula')
              }}
            >
              <Sigma />
              公式
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="hidden @min-[760px]:inline-flex"
              disabled={!editing}
              onClick={() => {
                setError('')
                setSelected('')
                setDialog('assets')
              }}
            >
              <ImagePlus />
              图片与附件
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" disabled={!editing}>
                  <Plus />
                  插入
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="start"
                portalContainer={
                  fullscreen ? (document.fullscreenElement as HTMLElement) : undefined
                }
              >
                {markdown && (
                  <DropdownMenuItem
                    onSelect={() => {
                      setError('')
                      setDialog('link')
                    }}
                  >
                    <Link2 />
                    链接
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem
                  onSelect={() => {
                    setError('')
                    setDialog('formula')
                  }}
                >
                  <Sigma />
                  公式
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() => {
                    setError('')
                    setSelected('')
                    setDialog('assets')
                  }}
                >
                  <ImagePlus />
                  图片与附件
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() => {
                    const marker = markdown ? '{{remainingsamples}}' : '\\remainingsamples'
                    const position = value.indexOf(marker)
                    if (position >= 0) commands.current?.jump(position)
                    else insert('\n\n' + marker + '\n\n')
                  }}
                >
                  <Braces />
                  样例
                </DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() =>
                    insert(
                      markdown
                        ? '\n\n```cpp\n// 代码\n```\n\n'
                        : '\n\\begin{verbatim}\n代码\n\\end{verbatim}\n',
                    )
                  }
                >
                  <Code2 />
                  代码块
                </DropdownMenuItem>
                {markdown && (
                  <DropdownMenuItem
                    onSelect={() =>
                      insert('\n\n| 项目 | 说明 |\n| --- | --- |\n| 内容 | 内容 |\n\n')
                    }
                  >
                    <Table2 />
                    表格
                  </DropdownMenuItem>
                )}
                {['题目描述', '输入格式', '输出格式', '样例说明', '数据范围与提示'].map((title) => (
                  <DropdownMenuItem
                    key={title}
                    onSelect={() =>
                      insert(markdown ? `\n\n## ${title}\n\n` : `\n\\section{${title}}\n`)
                    }
                  >
                    {title}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        )}
      </div>
      <div
        className={`relative flex min-h-0 ${fullscreen || focused ? 'flex-1' : 'h-[max(440px,65dvh)] max-h-[850px]'}`}
      >
        {outline && (
          <aside
            aria-label="题面章节"
            className="absolute inset-y-0 left-0 z-10 w-52 shrink-0 overflow-y-auto border-r bg-card p-3 shadow-lg md:static md:w-44 md:shadow-none"
          >
            <div className="mb-3 flex items-center justify-between">
              <span className="text-xs font-medium text-muted-foreground">
                章节 · {headings.length}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="size-7"
                aria-label="收起大纲"
                onClick={() => setOutline(false)}
              >
                <X />
              </Button>
            </div>
            {headings.length ? (
              headings.map((h) => (
                <button
                  key={h.from}
                  type="button"
                  className={`mb-1 block w-full truncate rounded-md px-2 py-2 text-left text-xs transition-colors hover:bg-accent hover:text-foreground ${mode !== 'preview' && currentHeading?.from === h.from ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground'}`}
                  aria-current={
                    mode !== 'preview' && currentHeading?.from === h.from ? 'location' : undefined
                  }
                  style={{ paddingLeft: Math.min(h.level - 1, 3) * 8 + 8 }}
                  title={h.title}
                  onClick={() => jumpToSection(h.from)}
                >
                  {h.title}
                </button>
              ))
            ) : (
              <p className="text-xs leading-6 text-muted-foreground">
                添加标题后，可在这里快速定位章节。
              </p>
            )}
          </aside>
        )}
        <SplitPane
          split={markdown && mode === 'split'}
          storageKey="vertex-authoring-statement-split"
          separatorLabel="调整题面源码与预览宽度"
          leftLabel="题面源码"
          className="min-h-0 min-w-0 flex-1"
          leftClassName={markdown && mode === 'preview' ? 'hidden' : 'flex'}
          rightClassName={markdown && mode !== 'edit' ? 'flex' : 'hidden'}
          left={
            <div className="flex h-full min-h-0 flex-col">
              {mode === 'split' && (
                <div className="border-b bg-muted/20 px-4 py-2 text-[11px] font-medium text-muted-foreground">
                  题面源码
                </div>
              )}
              <div className="min-h-0 flex-1">
                <CodeEditor
                  commands={commands}
                  value={value}
                  onChange={onChange}
                  onSelectionChange={setSelection}
                  language={markdown ? 'markdown' : 'tex'}
                  readOnly={readOnly || uploading}
                  ariaLabel="题面内容"
                  documentKey={entry.id}
                  className="rounded-none border-0 [&_.cm-content]:px-4 [&_.cm-content]:py-3 [&_.cm-line]:leading-[1.6] [&_.cm-scroller]:text-sm [&_.cm-gutters]:bg-transparent"
                />
              </div>
            </div>
          }
          right={
            markdown && mode !== 'edit' ? (
              <div className="flex h-full min-h-0 flex-col bg-background">
                <div className="flex items-center justify-between gap-2 border-b px-4 py-2 text-[11px] text-muted-foreground">
                  <span className="font-medium">题面预览</span>
                  {!readOnly && <span>双击内容定位源码</span>}
                </div>
                <section
                  ref={previewRef}
                  aria-label="题面预览"
                  tabIndex={0}
                  className="min-h-0 min-w-0 flex-1 overflow-auto"
                  onDoubleClick={(event) => {
                    if (
                      readOnly ||
                      !(event.target instanceof Element) ||
                      event.target.closest('a,button,input')
                    )
                      return
                    const block = event.target.closest<HTMLElement>('[data-source-line]')
                    if (!block || !event.currentTarget.contains(block)) return
                    const line = Number(block.dataset.sourceLine)
                    if (!Number.isInteger(line) || line < 0) return
                    const from = value
                      .split('\n')
                      .slice(0, line)
                      .reduce((offset, text) => offset + text.length + 1, 0)
                    setMode(wide ? 'split' : 'edit')
                    requestAnimationFrame(() => commands.current?.jump(from))
                  }}
                >
                  <div className="mx-auto max-w-3xl px-5 py-6 sm:px-8">
                    <StatementPreview
                      etag={etag}
                      revision={revision}
                      problemId={problemId}
                      entry={entry}
                      entries={entries}
                      content={value}
                    />
                  </div>
                </section>
              </div>
            ) : null
          }
        />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-2.5 text-xs text-muted-foreground">
        <span>
          {mode !== 'preview' && (
            <>
              行 {selection.line}，列 {selection.column} ·{' '}
            </>
          )}
          {selection.characters ? `选中 ${selection.characters.toLocaleString()} 字符 · ` : ''}
          {value.length.toLocaleString()} 字符
        </span>
        <span role="status">{status}</span>
      </div>
      <Dialog
        open={dialog !== null}
        onOpenChange={(open) => {
          if (!open && !uploading) setDialog(null)
        }}
      >
        <DialogContent
          portalContainer={fullscreen ? (document.fullscreenElement as HTMLElement) : undefined}
          className="max-w-3xl"
          onCloseAutoFocus={(event) => {
            event.preventDefault()
            commands.current?.focus()
          }}
        >
          <DialogTitle>
            {dialog === 'formula' ? '插入公式' : dialog === 'link' ? '插入链接' : '插入图片与附件'}
          </DialogTitle>
          <DialogDescription>
            {dialog === 'link'
              ? '添加一个可点击的参考链接。'
              : dialog === 'formula'
                ? '先查看排版效果，再插入当前光标位置。'
                : '选择公开附件，或直接上传。系统会自动关联到这份题面。'}
          </DialogDescription>
          {dialog === 'link' ? (
            <div className="space-y-4">
              <label className="block space-y-2 text-sm">
                <span>显示文字</span>
                <Input
                  aria-label="链接文字"
                  value={linkLabel}
                  onChange={(e) => setLinkLabel(e.target.value)}
                />
              </label>
              <label className="block space-y-2 text-sm">
                <span>链接地址</span>
                <Input
                  aria-label="链接地址"
                  placeholder="https://"
                  value={linkURL}
                  onChange={(e) => setLinkURL(e.target.value)}
                />
              </label>
              <div className="flex justify-end">
                <Button
                  onClick={() => {
                    try {
                      const url = new URL(linkURL)
                      if (!['https:', 'http:'].includes(url.protocol)) throw new Error()
                      const label = (linkLabel || url.hostname).replace(/[\\[\]]/g, '\\$&')
                      insert(`[${label}](${url.href.replace(/\(/g, '%28').replace(/\)/g, '%29')})`)
                    } catch {
                      setError('请输入有效的 http 或 https 链接。')
                    }
                  }}
                >
                  插入链接
                </Button>
              </div>
            </div>
          ) : dialog === 'formula' ? (
            <>
              <div className="flex gap-2">
                <Button
                  variant={blockMath ? 'secondary' : 'outline'}
                  aria-pressed={blockMath}
                  onClick={() => setBlockMath(true)}
                >
                  独立公式
                </Button>
                <Button
                  variant={!blockMath ? 'secondary' : 'outline'}
                  aria-pressed={!blockMath}
                  onClick={() => setBlockMath(false)}
                >
                  行内公式
                </Button>
              </div>
              <label htmlFor="statement-formula" className="text-sm font-medium">
                LaTeX 公式
              </label>
              <Textarea
                id="statement-formula"
                className="min-h-28 resize-none font-mono"
                value={formula}
                onChange={(e) => setFormula(e.target.value)}
              />
              <div className="min-h-24 overflow-auto rounded-lg border bg-background px-5 py-4">
                <MdRenderer content={`$$\n${formula}\n$$`} />
              </div>
              <div className="flex justify-end">
                <Button
                  disabled={!formula.trim()}
                  onClick={() =>
                    insert(
                      blockMath
                        ? `\n\n${markdown ? '$$' : '\\['}\n${formula}\n${markdown ? '$$' : '\\]'}\n\n`
                        : `$${formula}$`,
                    )
                  }
                >
                  插入公式
                </Button>
              </div>
            </>
          ) : (
            <>
              <div className="flex gap-2">
                <Input
                  aria-label="搜索附件"
                  placeholder="按名称搜索"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <input
                  ref={upload}
                  type="file"
                  className="hidden"
                  aria-label="上传题面附件"
                  accept={markdown ? undefined : '.png,.jpg,.jpeg,.pdf'}
                  onChange={(e) => {
                    if (e.target.files?.[0]) void add(e.target.files[0])
                  }}
                />
                <Button
                  variant="outline"
                  loading={uploading}
                  onClick={() => upload.current?.click()}
                >
                  <Upload />
                  上传
                </Button>
              </div>
              <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_220px]">
                <div
                  className="max-h-72 min-h-32 space-y-1 overflow-y-auto rounded-lg border p-2"
                  aria-label="可用附件"
                >
                  {visible.map((file) => (
                    <button
                      key={file.id}
                      type="button"
                      disabled={uploading}
                      aria-pressed={selected === file.id}
                      onClick={() => setSelected(file.id)}
                      className={`flex w-full items-center gap-3 rounded-lg border px-3 py-3 text-left transition-colors ${selected === file.id ? 'border-primary/40 bg-primary/10' : 'border-transparent hover:bg-muted'}`}
                    >
                      <File className="size-5 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate text-sm">{entryLabel(file)}</span>
                      <span className="text-xs text-muted-foreground">
                        {formatFileSize(file.blob.bytes)}
                      </span>
                    </button>
                  ))}
                  {!visible.length && (
                    <p className="py-8 text-center text-sm text-muted-foreground">
                      {query ? '没有匹配的附件' : '还没有可插入的附件，上传后即可选择。'}
                    </p>
                  )}
                </div>
                <AttachmentPreview
                  problemId={problemId}
                  file={files.find((f) => f.id === selected)}
                />
              </div>
              <p className="text-xs text-muted-foreground">
                这里只显示选手可见的附件。内部资料和测试数据不会出现在这里。
              </p>
              <div className="flex justify-end">
                <Button
                  disabled={uploading || !files.some((f) => f.id === selected)}
                  onClick={() => {
                    const file = files.find((f) => f.id === selected)
                    if (file) insertAsset(file)
                  }}
                >
                  插入所选附件
                </Button>
              </div>
            </>
          )}
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
