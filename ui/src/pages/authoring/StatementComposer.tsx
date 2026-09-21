import { useMemo, useRef, useState, type ReactNode } from 'react'
import {
  Bold,
  Italic,
  Heading2,
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
} from 'lucide-react'
import type { DomainTreeEntry } from '@/generated/api/model'
import CodeEditor, { type EditorCommands } from '@/components/CodeEditor'
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

export default function StatementComposer({
  navigation,
  actions,
  problemId,
  entry,
  entries,
  value,
  onChange,
  readOnly,
  fullscreen,
  status,
  onUpload,
}: {
  navigation?: ReactNode
  actions?: ReactNode
  problemId: string
  entry: DomainTreeEntry
  entries: DomainTreeEntry[]
  value: string
  onChange: (value: string) => void
  readOnly: boolean
  fullscreen: boolean
  status: string
  onUpload: (file: File) => Promise<DomainTreeEntry>
}) {
  const commands = useRef<EditorCommands | null>(null),
    upload = useRef<HTMLInputElement>(null)
  const markdown = entry.attributes.format === 'markdown'
  const [mode, setMode] = useState<'edit' | 'split' | 'preview'>(() =>
    window.matchMedia('(min-width:1024px)').matches ? 'split' : 'edit',
  )
  const [outline, setOutline] = useState(false),
    [dialog, setDialog] = useState<'assets' | 'formula' | null>(null)
  const [query, setQuery] = useState(''),
    [selected, setSelected] = useState(''),
    [error, setError] = useState(''),
    [uploading, setUploading] = useState(false)
  const [formula, setFormula] = useState('a^2 + b^2 = c^2'),
    [blockMath, setBlockMath] = useState(true)
  const headings = useMemo(() => statementOutline(value, !markdown), [value, markdown])
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
    <div className="overflow-hidden rounded-xl border bg-card">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          {navigation}
          <span className="text-xs text-muted-foreground">{markdown ? 'Markdown' : 'TeX'}</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {markdown ? (
            <div className="flex rounded-lg border bg-background p-0.5" aria-label="题面显示方式">
              {(['edit', 'split', 'preview'] as const).map((item) => (
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
          {actions}
        </div>
      </div>
      <div
        className="flex items-center gap-1 overflow-x-auto border-b bg-muted/20 px-3 py-1.5 [&>*]:shrink-0"
        aria-label="题面格式工具"
      >
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label="题面大纲"
            title="题面大纲"
            aria-pressed={outline}
            onClick={() => setOutline(!outline)}
          >
            <ListTree />
          </Button>
          <span className="mr-1 h-5 border-l" />
          <Button
            variant="ghost"
            size="icon"
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
            disabled={mode === 'preview'}
            onClick={() => commands.current?.search()}
          >
            <Search />
          </Button>
        </div>
        <span className="mx-1 h-5 border-l" />
        {(
          [
            { id: 'heading', label: '标题', icon: Heading2 },
            { id: 'bold', label: '加粗', icon: Bold },
            { id: 'italic', label: '斜体', icon: Italic },
            { id: 'list', label: '列表', icon: List },
            { id: 'quote', label: '引用', icon: Quote },
          ] as const
        ).map(({ id, label, icon: Icon }) => (
          <Button
            key={id}
            variant="ghost"
            size="icon"
            aria-label={label}
            title={label}
            disabled={!editing}
            onClick={() => commands.current?.format(id, !markdown)}
          >
            <Icon />
          </Button>
        ))}
        <span className="mx-1 h-5 border-l" />
        <Button
          variant="ghost"
          size="sm"
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
            portalContainer={fullscreen ? (document.fullscreenElement as HTMLElement) : undefined}
          >
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
                onSelect={() => insert('\n\n| 项目 | 说明 |\n| --- | --- |\n| 内容 | 内容 |\n\n')}
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
      </div>
      <div
        className={`relative flex min-h-0 ${fullscreen ? 'h-[calc(100dvh-17rem)] min-h-72' : 'h-[max(440px,65dvh)] max-h-[850px]'}`}
      >
        {outline && (
          <aside
            aria-label="题面章节"
            className="absolute inset-y-0 left-0 z-10 w-52 shrink-0 overflow-y-auto border-r bg-card p-3 shadow-lg md:static md:w-44 md:shadow-none"
          >
            <div className="mb-3 flex items-center justify-between">
              <span className="text-xs font-medium text-muted-foreground">文档大纲</span>
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
                  className="mb-1 block w-full truncate rounded-md px-2 py-2 text-left text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                  style={{ paddingLeft: Math.min(h.level - 1, 3) * 8 + 8 }}
                  title={h.title}
                  onClick={() => {
                    if (mode === 'preview') setMode('edit')
                    requestAnimationFrame(() => commands.current?.jump(h.from))
                    if (window.innerWidth < 768) setOutline(false)
                  }}
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
        <div
          className={`grid min-h-0 min-w-0 flex-1 ${markdown && mode === 'split' ? 'grid-rows-2 lg:grid-cols-2 lg:grid-rows-1' : 'grid-cols-1'}`}
        >
          <div className={`min-h-0 min-w-0 ${markdown && mode === 'preview' ? 'hidden' : ''}`}>
            <CodeEditor
              commands={commands}
              value={value}
              onChange={onChange}
              language={markdown ? 'markdown' : 'tex'}
              readOnly={readOnly || uploading}
              ariaLabel="题面内容"
              documentKey={entry.id}
              className="rounded-none border-0 [&_.cm-content]:px-4 [&_.cm-content]:py-5 [&_.cm-line]:leading-7 [&_.cm-scroller]:text-sm [&_.cm-gutters]:bg-transparent"
            />
          </div>
          {markdown && mode !== 'edit' && (
            <section
              aria-label="题面预览"
              className={`min-h-0 min-w-0 overflow-auto bg-background ${mode === 'split' ? 'border-t lg:border-l lg:border-t-0' : ''}`}
            >
              <div className="mx-auto max-w-3xl px-5 py-6 sm:px-8">
                <StatementPreview
                  problemId={problemId}
                  entry={entry}
                  entries={entries}
                  content={value}
                />
              </div>
            </section>
          )}
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-2.5 text-xs text-muted-foreground">
        <span>
          {markdown ? 'Markdown' : 'TeX'} · {value.length.toLocaleString()} 字符
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
          }}
        >
          <DialogTitle>{dialog === 'formula' ? '插入公式' : '插入图片与附件'}</DialogTitle>
          <DialogDescription>
            {dialog === 'formula'
              ? '先查看排版效果，再插入当前光标位置。'
              : '选择公开附件，或直接上传。系统会自动关联到这份题面。'}
          </DialogDescription>
          {dialog === 'formula' ? (
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
