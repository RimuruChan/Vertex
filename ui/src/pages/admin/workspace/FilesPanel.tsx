import { useEffect, useState } from 'react'
import { FileCode2, Plus, Save, Sparkles, Star, Trash2 } from 'lucide-react'
import {
  deleteApiAdminProblemsIdFilesFileId as deleteFile,
  getApiAdminPackageTemplates as listTemplates,
  getApiAdminProblemsIdFilesFileId as getFile,
  putApiAdminProblemsIdFiles as saveFile,
} from '@/generated/api/vertex'
import type { DtoTemplateResponse } from '@/generated/api/model'
import CodeEditor from '@/components/CodeEditor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState } from '@/components/ui/misc'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'
import {
  EXPECTED_VERDICTS,
  FILE_KINDS,
  KIND_HINTS,
  KIND_LABELS,
  KIND_LANGUAGES,
  LANGUAGE_LABELS,
  type FileKind,
  type PackageFile,
} from './types'

type Draft = {
  id?: number
  kind: FileKind
  name: string
  language: string
  sourceCode: string
  expectedVerdict: string
  isActive: boolean
}

function defaultName(kind: FileKind): string {
  switch (kind) {
    case 'checker':
      return 'check'
    case 'validator':
      return 'validate'
    case 'generator':
      return 'gen'
    case 'interactor':
      return 'interact'
    default:
      return 'std'
  }
}

function newDraft(kind: FileKind): Draft {
  return {
    kind,
    name: defaultName(kind),
    language: KIND_LANGUAGES[kind][0],
    sourceCode: '',
    expectedVerdict: kind === 'solution' ? 'Accepted' : '',
    isActive: kind !== 'generator' && kind !== 'solution',
  }
}

function toDraft(file: PackageFile): Draft {
  return {
    id: file.id,
    kind: file.kind as FileKind,
    name: file.name,
    language: file.language,
    sourceCode: file.sourceCode,
    expectedVerdict: file.expectedVerdict,
    isActive: file.isActive,
  }
}

/**
 * Source editing for the whole package. Files are listed by role and opened
 * one at a time, because a package's sources are large and the list endpoint
 * deliberately omits their bodies.
 */
export default function FilesPanel({
  problemId,
  files,
  onChanged,
}: {
  problemId: string
  files: PackageFile[]
  onChanged: () => void
}) {
  const toast = useToast()
  const confirm = useConfirm()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [loadingFile, setLoadingFile] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [templates, setTemplates] = useState<DtoTemplateResponse[]>([])
  const [templatesOpen, setTemplatesOpen] = useState(false)

  useEffect(() => {
    listTemplates()
      .then((result) => setTemplates(result.items))
      .catch(() => setTemplates([]))
  }, [])

  async function openFile(file: PackageFile) {
    setLoadingFile(file.id)
    try {
      const full = await getFile(problemId, file.id)
      setDraft(toDraft(full))
    } catch (error) {
      toast.error(apiError(error, '打开文件失败'))
    } finally {
      setLoadingFile(null)
    }
  }

  async function handleSave() {
    if (!draft) return
    if (!draft.sourceCode.trim()) {
      toast.warning('源码不能为空')
      return
    }
    setSaving(true)
    try {
      await saveFile(problemId, {
        kind: draft.kind,
        name: draft.name,
        language: draft.language,
        sourceCode: draft.sourceCode,
        expectedVerdict: draft.kind === 'solution' ? draft.expectedVerdict : undefined,
        isActive: draft.isActive,
      })
      toast.success(`${KIND_LABELS[draft.kind]} ${draft.name} 已保存`)
      onChanged()
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(file: PackageFile) {
    if (deletingId !== null) return
    const accepted = await confirm({
      title: `删除${KIND_LABELS[file.kind as FileKind]}「${file.name}」？`,
      description:
        '源码会从题目包草稿中永久删除，无法撤销。当前已发布的测试包会保持不变，直到下一次成功构建。',
      confirmLabel: '删除文件',
      destructive: true,
    })
    if (!accepted) return
    setDeletingId(file.id)
    try {
      await deleteFile(problemId, file.id)
      toast.success('已删除')
      if (draft?.id === file.id) setDraft(null)
      onChanged()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setDeletingId(null)
    }
  }

  function applyTemplate(template: DtoTemplateResponse) {
    setDraft({
      kind: template.kind as FileKind,
      name: template.name,
      language: template.language,
      sourceCode: template.sourceCode,
      expectedVerdict: template.kind === 'solution' ? 'Accepted' : '',
      isActive: template.kind !== 'generator' && template.kind !== 'solution',
    })
    setTemplatesOpen(false)
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,20rem)_1fr]">
      <Card className="flex flex-col gap-3 p-4">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium">题目包文件</p>
          <Button size="sm" variant="outline" onClick={() => setTemplatesOpen(true)}>
            <Sparkles />
            模板
          </Button>
        </div>

        {FILE_KINDS.map((kind) => {
          const group = files.filter((file) => file.kind === kind)
          return (
            <div key={kind} className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {KIND_LABELS[kind]}
                </span>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  aria-label={`新建 ${KIND_LABELS[kind]}`}
                  onClick={() => setDraft(newDraft(kind))}
                >
                  <Plus />
                </Button>
              </div>
              {group.length === 0 ? (
                <p className="text-xs text-muted-foreground">尚未添加</p>
              ) : (
                group.map((file) => (
                  <div key={file.id} className="flex items-center gap-1">
                    <Button
                      variant={draft?.id === file.id ? 'secondary' : 'ghost'}
                      size="sm"
                      className="flex-1 justify-start"
                      loading={loadingFile === file.id}
                      onClick={() => openFile(file)}
                    >
                      <FileCode2 />
                      <span className="truncate">{file.name}</span>
                      {file.isActive ? <Star className="size-3 text-amber-500" /> : null}
                      <Badge variant="secondary" className="ml-auto">
                        {LANGUAGE_LABELS[file.language] ?? file.language}
                      </Badge>
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`删除 ${file.name}`}
                      className="hover:text-destructive"
                      disabled={deletingId !== null}
                      loading={deletingId === file.id}
                      onClick={() => handleDelete(file)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                ))
              )}
            </div>
          )
        })}
      </Card>

      <Card className="flex min-h-[32rem] flex-col gap-3 p-4">
        {draft === null ? (
          <EmptyState
            icon={<FileCode2 />}
            title="选择或新建一个文件"
            description="标程、checker、validator 和生成器都在这里编辑,保存后触发构建才会生效。"
          />
        ) : (
          <>
            <p className="text-xs text-muted-foreground">{KIND_HINTS[draft.kind]}</p>
            <div className="grid gap-3 sm:grid-cols-4">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="file-kind">类型</Label>
                <Select
                  value={draft.kind}
                  onValueChange={(value) => {
                    const kind = value as FileKind
                    setDraft({
                      ...draft,
                      kind,
                      language: KIND_LANGUAGES[kind].includes(draft.language)
                        ? draft.language
                        : KIND_LANGUAGES[kind][0],
                      isActive: kind === 'generator' ? false : draft.isActive,
                    })
                  }}
                >
                  <SelectTrigger id="file-kind">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {FILE_KINDS.map((kind) => (
                      <SelectItem key={kind} value={kind}>
                        {KIND_LABELS[kind]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="file-name">名称</Label>
                <Input
                  id="file-name"
                  value={draft.name}
                  onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                  placeholder="gen_random"
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="file-language">语言</Label>
                <Select
                  value={draft.language}
                  onValueChange={(value) => setDraft({ ...draft, language: value })}
                >
                  <SelectTrigger id="file-language">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {KIND_LANGUAGES[draft.kind].map((language) => (
                      <SelectItem key={language} value={language}>
                        {LANGUAGE_LABELS[language] ?? language}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {draft.kind === 'solution' ? (
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="file-verdict">预期判定</Label>
                  <Select
                    value={draft.isActive ? 'Accepted' : draft.expectedVerdict || 'Accepted'}
                    disabled={draft.isActive}
                    onValueChange={(value) => setDraft({ ...draft, expectedVerdict: value })}
                  >
                    <SelectTrigger id="file-verdict">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {EXPECTED_VERDICTS.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ) : null}
            </div>

            {draft.kind === 'solution' ? (
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="size-4 accent-primary"
                  checked={draft.isActive}
                  onChange={(event) => setDraft({ ...draft, isActive: event.target.checked })}
                />
                设为标程(用它产生每个测试点的答案)
              </label>
            ) : null}

            <div className="min-h-[24rem] flex-1 overflow-hidden rounded-md border border-border">
              <CodeEditor
                value={draft.sourceCode}
                language={draft.language}
                onChange={(value) => setDraft({ ...draft, sourceCode: value })}
              />
            </div>

            <div className="flex items-center gap-2">
              <Button loading={saving} onClick={handleSave}>
                <Save />
                保存
              </Button>
              <Button variant="ghost" onClick={() => setDraft(null)}>
                关闭
              </Button>
            </div>
          </>
        )}
      </Card>

      <Dialog open={templatesOpen} onOpenChange={setTemplatesOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>从模板新建</DialogTitle>
          </DialogHeader>
          <div className="flex max-h-[60vh] flex-col gap-2 overflow-y-auto">
            {templates.map((template, index) => (
              <button
                key={`${template.kind}-${index}`}
                type="button"
                className="flex flex-col gap-1 rounded-md border border-border p-3 text-left hover:bg-accent"
                onClick={() => applyTemplate(template)}
              >
                <div className="flex items-center gap-2">
                  <Badge variant="secondary">{KIND_LABELS[template.kind] ?? template.kind}</Badge>
                  <span className="text-sm font-medium">{template.title}</span>
                </div>
                <span className="text-xs text-muted-foreground">{template.description}</span>
              </button>
            ))}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
