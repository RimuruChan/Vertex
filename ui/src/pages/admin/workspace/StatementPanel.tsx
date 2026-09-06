import { useEffect, useMemo, useState } from 'react'
import { Eye, Plus, Save, Trash2 } from 'lucide-react'
import {
  deleteApiAdminProblemsIdStatementsLanguage as deleteStatement,
  postApiAdminProblemsIdStatementsLanguagePreview as previewStatement,
  putApiAdminProblemsIdStatementsLanguage as saveStatement,
} from '@/generated/api/vertex'
import MdRenderer from '@/components/MdRenderer'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'
import type { PackageStatement } from './types'

type Draft = {
  name: string
  legend: string
  inputFormat: string
  outputFormat: string
  notes: string
  scoring: string
  tutorial: string
}

const emptyDraft: Draft = {
  name: '',
  legend: '',
  inputFormat: '',
  outputFormat: '',
  notes: '',
  scoring: '',
  tutorial: '',
}

function toDraft(statement: PackageStatement | undefined): Draft {
  if (!statement) return emptyDraft
  return {
    name: statement.name,
    legend: statement.legend,
    inputFormat: statement.inputFormat,
    outputFormat: statement.outputFormat,
    notes: statement.notes,
    scoring: statement.scoring,
    tutorial: statement.tutorial,
  }
}

const SECTIONS: { key: keyof Draft; label: string; hint: string; rows: number }[] = [
  { key: 'legend', label: '题目描述', hint: '支持 Markdown 与 $LaTeX$', rows: 12 },
  { key: 'inputFormat', label: '输入格式', hint: '描述输入的结构与数据范围', rows: 6 },
  { key: 'outputFormat', label: '输出格式', hint: '描述输出的结构', rows: 6 },
  { key: 'notes', label: '说明与提示', hint: '样例解释、数据范围补充', rows: 6 },
  { key: 'scoring', label: '计分方式', hint: '子任务/部分分说明,留空则不显示', rows: 4 },
  { key: 'tutorial', label: '题解(不公开)', hint: '仅出题人可见的解题思路', rows: 6 },
]

/**
 * Statement editing is structured rather than free-form Markdown: the public
 * page is rendered from these sections plus the samples the last build
 * produced, so an author can never show an example the judge would reject.
 */
export default function StatementPanel({
  problemId,
  statements,
  primaryLanguage,
  onSaved,
}: {
  problemId: string
  statements: PackageStatement[]
  primaryLanguage: string
  onSaved: () => void
}) {
  const toast = useToast()
  const confirm = useConfirm()
  const languages = useMemo(() => {
    const known = statements.map((item) => item.language)
    return known.includes(primaryLanguage) ? known : [primaryLanguage, ...known]
  }, [statements, primaryLanguage])

  const [language, setLanguage] = useState(primaryLanguage)
  const [draft, setDraft] = useState<Draft>(() =>
    toDraft(statements.find((item) => item.language === primaryLanguage)),
  )
  const [saving, setSaving] = useState(false)
  const [preview, setPreview] = useState<string | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [newLanguage, setNewLanguage] = useState('')

  useEffect(() => {
    setDraft(toDraft(statements.find((item) => item.language === language)))
    setPreview(null)
  }, [language, statements])

  async function handleSave() {
    setSaving(true)
    try {
      await saveStatement(problemId, language, draft)
      toast.success('题面已保存')
      onSaved()
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  async function handlePreview() {
    setPreviewing(true)
    try {
      const result = await previewStatement(problemId, language, draft)
      setPreview(result.statementMd)
    } catch (error) {
      toast.error(apiError(error, '预览失败'))
    } finally {
      setPreviewing(false)
    }
  }

  async function handleDelete() {
    if (deleting) return
    const accepted = await confirm({
      title: `删除 ${language} 题面？`,
      description:
        '该语言下已保存的题目描述、输入输出格式、说明与内部题解都会永久删除，无法撤销。主语言的公开题面不会因此改变。',
      confirmLabel: '删除题面',
      destructive: true,
    })
    if (!accepted) return
    setDeleting(true)
    try {
      await deleteStatement(problemId, language)
      toast.success('已删除该语言的题面')
      setLanguage(primaryLanguage)
      onSaved()
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setDeleting(false)
    }
  }

  function addLanguage() {
    const value = newLanguage.trim().toLowerCase()
    if (!value) return
    setNewLanguage('')
    setLanguage(value)
    setDraft(emptyDraft)
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_minmax(0,26rem)]">
      <Card className="flex flex-col gap-4 p-4">
        <div className="flex flex-wrap items-center gap-2">
          {languages.map((item) => (
            <Button
              key={item}
              size="sm"
              variant={item === language ? 'default' : 'outline'}
              onClick={() => setLanguage(item)}
            >
              {item}
              {item === primaryLanguage ? (
                <Badge variant="secondary" className="ml-1">
                  公开
                </Badge>
              ) : null}
            </Button>
          ))}
          <div className="flex items-center gap-1">
            <Input
              value={newLanguage}
              onChange={(event) => setNewLanguage(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  addLanguage()
                }
              }}
              placeholder="en"
              className="h-8 w-20"
              aria-label="新增语言"
            />
            <Button size="icon-sm" variant="ghost" onClick={addLanguage} aria-label="新增语言">
              <Plus />
            </Button>
          </div>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="statement-name">题目名称</Label>
          <Input
            id="statement-name"
            value={draft.name}
            onChange={(event) => setDraft({ ...draft, name: event.target.value })}
            placeholder="保存后会同步为题目标题"
          />
        </div>

        {SECTIONS.map((section) => (
          <div key={section.key} className="flex flex-col gap-1.5">
            <div className="flex items-baseline justify-between">
              <Label htmlFor={`statement-${section.key}`}>{section.label}</Label>
              <span className="text-xs text-muted-foreground">{section.hint}</span>
            </div>
            <Textarea
              id={`statement-${section.key}`}
              rows={section.rows}
              value={draft[section.key]}
              onChange={(event) => setDraft({ ...draft, [section.key]: event.target.value })}
            />
          </div>
        ))}

        <div className="flex flex-wrap items-center gap-2">
          <Button loading={saving} onClick={handleSave}>
            <Save />
            保存题面
          </Button>
          <Button variant="outline" loading={previewing} onClick={handlePreview}>
            <Eye />
            预览公开题面
          </Button>
          {language !== primaryLanguage ? (
            <Button
              variant="ghost"
              className="hover:text-destructive"
              loading={deleting}
              onClick={handleDelete}
            >
              <Trash2 />
              删除该语言
            </Button>
          ) : null}
        </div>
      </Card>

      <Card className="flex max-h-[80vh] flex-col gap-2 overflow-y-auto p-4">
        <p className="text-sm font-medium">公开题面预览</p>
        <p className="text-xs text-muted-foreground">
          样例来自最近一次成功构建的样例测试点,重新构建后会自动更新。
        </p>
        {preview ? (
          <MdRenderer content={preview} />
        ) : (
          <p className="py-8 text-center text-sm text-muted-foreground">
            点击「预览公开题面」查看渲染结果。
          </p>
        )}
      </Card>
    </div>
  )
}
