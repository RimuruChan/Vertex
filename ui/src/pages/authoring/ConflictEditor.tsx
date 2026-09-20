import { useEffect, useState } from 'react'
import type { DomainContentConflict, DomainTreeEntry } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Input, Textarea } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Field } from './MaterialForm'
import { apiError } from '@/lib/format'
import { isDocument } from '@/lib/authoring-materials'

export default function ConflictEditor({
  problemId,
  conflict,
  entry,
  disabled,
  onResolve,
  onDirty,
}: {
  problemId: string
  conflict: DomainContentConflict
  entry?: DomainTreeEntry
  disabled: boolean
  onResolve: (entry: DomainTreeEntry) => Promise<void>
  onDirty: (key: string, dirty: boolean) => void
}) {
  const api = useDomainAPI(),
    [values, setValues] = useState<(string | undefined)[]>([]),
    [error, setError] = useState('')
  const [opened, setOpened] = useState(false),
    [loading, setLoading] = useState(false),
    [saving, setSaving] = useState(false)
  const [edited, setEdited] = useState(false)
  const key = `${conflict.entryId}:${conflict.field}`
  useEffect(() => {
    onDirty(key, edited)
    return () => onDirty(key, false)
  }, [key, edited, onDirty])
  const [text, setText] = useState(''),
    [path, setPath] = useState(entry?.path ?? conflict.local?.path ?? conflict.remote?.path ?? ''),
    [attribute, setAttribute] = useState('')
  const selected = entry ?? conflict.local ?? conflict.remote ?? conflict.base
  useEffect(() => {
    // Another conflict can be saved while this editor still has local input.
    // Refreshing its selected entry must not overwrite that unfinished draft.
    if (!opened || edited) return
    let live = true
    setLoading(true)
    setError('')
    const read = async (item?: DomainTreeEntry) => {
      if (!item) return '(此版本不存在)'
      if (item.blob.bytes > 128 * 1024 || item.kind === 'asset' || item.attributes.format === 'pdf')
        return undefined
      const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, item.blob.sha256)
      try {
        const value = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
        if (value.includes('\0')) return undefined
        return isDocument(item.kind) ? JSON.stringify(JSON.parse(value), null, 2) : value
      } catch {
        return undefined
      }
    }
    Promise.all([read(conflict.base), read(conflict.local), read(conflict.remote), read(selected)])
      .then((result) => {
        if (!live) return
        setValues(result.slice(0, 3))
        setText(result[3] ?? '')
        setAttribute(selected?.attributes[conflict.field.slice(11)] ?? '')
      })
      .catch((error) => {
        if (live) setError(apiError(error, '读取冲突内容失败'))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [api, problemId, conflict, selected, opened, edited])
  async function resolve() {
    if (!selected) return
    setSaving(true)
    setError('')
    try {
      const next = { ...selected, id: conflict.entryId, attributes: { ...selected.attributes } }
      if (conflict.field === 'path') next.path = path.trim()
      else if (conflict.field.startsWith('attributes.')) {
        const key = conflict.field.slice(11)
        if (attribute) next.attributes[key] = attribute
        else delete next.attributes[key]
      } else {
        if (isDocument(next.kind)) JSON.parse(text)
        next.blob = await api.postApiAuthoringProblemsIdBlobs(problemId, {
          file: new Blob([text], { type: 'text/plain;charset=utf-8' }),
        })
        if (conflict.field === 'entry') next.path = path.trim()
      }
      await onResolve(next)
      setEdited(false)
    } catch (error) {
      setError(apiError(error, '无法保存合并结果，请检查格式和路径'))
    } finally {
      setSaving(false)
    }
  }
  const content = conflict.field === 'blob' || conflict.field === 'entry'
  const editable =
    (selected && ['blob', 'entry', 'path'].includes(conflict.field)) ||
    conflict.field.startsWith('attributes.')
  const binary =
    selected &&
    (selected.blob.bytes > 128 * 1024 ||
      selected.kind === 'asset' ||
      selected.attributes.format === 'pdf' ||
      values.some(
        (value, index) =>
          [conflict.base, conflict.local, conflict.remote][index] && value === undefined,
      ))
  return (
    <div className="space-y-3">
      <Button
        variant="outline"
        disabled={disabled || edited}
        onClick={() => setOpened((value) => !value)}
      >
        {opened ? '收起手工合并' : '查看基线并手工合并'}
      </Button>
      {opened && (
        <>
          {loading ? (
            <p className="text-sm text-muted-foreground">读取三个版本…</p>
          ) : (
            <div className="grid min-w-0 gap-3 xl:grid-cols-3">
              {['共同基线', '我的副本', '最新提交'].map((label, index) => (
                <div key={label} className="min-w-0 rounded-lg border">
                  <h4 className="border-b px-3 py-2 text-xs font-medium">{label}</h4>
                  <p className="break-all border-b px-3 py-2 text-xs text-muted-foreground">
                    {[conflict.base, conflict.local, conflict.remote][index]?.path ?? '不存在'}
                  </p>
                  <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words p-3 text-xs leading-5">
                    {values[index] ??
                      '此文件保留二进制或大文件形式，请选择保留一方，或完成合并后替换材料。'}
                  </pre>
                </div>
              ))}
            </div>
          )}
          {editable && !loading && (
            <>
              {(conflict.field === 'path' || conflict.field === 'entry') && (
                <Field label="合并后的文件路径">
                  <Input
                    aria-label="合并后的文件路径"
                    value={path}
                    onChange={(event) => {
                      setPath(event.target.value)
                      setEdited(true)
                    }}
                    disabled={saving || disabled}
                  />
                </Field>
              )}
              {conflict.field.startsWith('attributes.') && (
                <Field label={`属性 ${conflict.field.slice(11)}`}>
                  <Input
                    aria-label="合并后的属性"
                    value={attribute}
                    onChange={(event) => {
                      setAttribute(event.target.value)
                      setEdited(true)
                    }}
                    disabled={saving || disabled}
                  />
                </Field>
              )}
              {content && !binary && (
                <Field label="合并后的内容">
                  <Textarea
                    aria-label="手工合并内容"
                    className="min-h-64 font-mono text-xs leading-6"
                    value={text}
                    onChange={(event) => {
                      setText(event.target.value)
                      setEdited(true)
                    }}
                    disabled={saving || disabled}
                  />
                </Field>
              )}
              {(!content || !binary) && (
                <Button
                  loading={saving}
                  disabled={saving || disabled}
                  onClick={() => void resolve()}
                >
                  保存此项合并结果
                </Button>
              )}
            </>
          )}
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
        </>
      )}
    </div>
  )
}
