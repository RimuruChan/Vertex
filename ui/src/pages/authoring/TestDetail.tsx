import { useEffect, useRef, useState } from 'react'
import type { DomainTestMaterial, DomainTreeEntry, DomainWorkingCopy } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { SaveButton } from '@/components/ui/save-button'
import { Download, Upload } from 'lucide-react'
import { Field } from './FormFields'
import { saveTestDrafts } from './data-actions'
import { apiError, formatFileSize } from '@/lib/format'

export default function TestDetail({
  problemId,
  entry,
  copy,
  canEdit,
  onSaved,
  onDirty,
  onBusy,
  onAdvanced,
}: {
  problemId: string
  entry: DomainTreeEntry
  copy: DomainWorkingCopy
  canEdit: boolean
  onSaved: (copy: DomainWorkingCopy) => void
  onDirty: (dirty: boolean) => void
  onBusy: (busy: boolean) => void
  onAdvanced: () => void
}) {
  const api = useDomainAPI(),
    [definition, setDefinition] = useState<DomainTestMaterial>(),
    [original, setOriginal] = useState(''),
    [text, setText] = useState<{ input?: string; answer?: string }>({}),
    [originalText, setOriginalText] = useState<{ input?: string; answer?: string }>({}),
    [files, setFiles] = useState<{ input?: File; answer?: File }>({}),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [saved, setSaved] = useState(false)
  const inputs = useRef<{ input: HTMLInputElement | null; answer: HTMLInputElement | null }>({
    input: null,
    answer: null,
  })
  const dirty =
    !!definition &&
    (JSON.stringify(definition) !== original ||
      JSON.stringify(text) !== JSON.stringify(originalText) ||
      !!files.input ||
      !!files.answer)
  useEffect(() => {
    onDirty(dirty)
    return () => onDirty(false)
  }, [dirty, onDirty])
  useEffect(() => {
    onBusy(busy)
    return () => onBusy(false)
  }, [busy, onBusy])
  useEffect(() => {
    let live = true
    setLoading(true)
    void (async () => {
      const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, entry.blob.sha256),
        value = JSON.parse(await blob.text()) as DomainTestMaterial
      const previews: { input?: string; answer?: string } = {}
      for (const part of ['input', 'answer'] as const) {
        const file = copy.tree.entries.find((e) => e.id === value[part].entry)
        if (file && file.blob.bytes <= 128 * 1024) {
          try {
            const content = await api.getApiAuthoringProblemsIdBlobsDigest(
              problemId,
              file.blob.sha256,
            )
            const decoded = new TextDecoder('utf-8', { fatal: true }).decode(
              await content.arrayBuffer(),
            )
            if (!decoded.includes('\0')) previews[part] = decoded
          } catch {}
        }
      }
      if (live) {
        setDefinition(value)
        setOriginal(JSON.stringify(value))
        setText(previews)
        setOriginalText(previews)
        setFiles({})
        setError('')
      }
    })()
      .catch((e) => {
        if (live) setError(apiError(e, '测试数据加载失败'))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [api, problemId, entry.id, entry.blob.sha256])
  async function save() {
    if (!definition) return
    setBusy(true)
    setError('')
    setSaved(false)
    try {
      const replacement = { ...files }
      for (const part of ['input', 'answer'] as const)
        if (!replacement[part] && text[part] !== undefined && text[part] !== originalText[part])
          replacement[part] = new File([text[part]!], part === 'input' ? 'input.in' : 'answer.ans')
      const next = await saveTestDrafts(api, problemId, copy, [
        { entry, definition, ...replacement },
      ])
      setOriginal(JSON.stringify(definition))
      setOriginalText(text)
      setFiles({})
      setSaved(true)
      onSaved(next)
    } catch (e) {
      setError(apiError(e, '保存失败，输入内容已保留'))
    } finally {
      setBusy(false)
    }
  }
  async function download(file: DomainTreeEntry) {
    try {
      const blob = await api.getApiAuthoringProblemsIdBlobsDigest(problemId, file.blob.sha256),
        url = URL.createObjectURL(blob),
        link = document.createElement('a')
      link.href = url
      link.download = file.attributes.label || file.path.split('/').pop()!
      link.click()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setError(apiError(e, '下载失败'))
    }
  }
  if (loading && !definition)
    return <p className="py-8 text-sm text-muted-foreground">正在读取测试点…</p>
  return (
    <div className="space-y-5">
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {definition && (
        <fieldset disabled={busy || loading} className="min-w-0 space-y-5">
          <div className="grid items-end gap-4 sm:grid-cols-[minmax(0,1fr)_auto]">
            <Field label="测试点名称">
              <Input
                aria-label="测试点名称"
                disabled={!canEdit}
                value={definition.name}
                onChange={(e) => {
                  setDefinition({ ...definition, name: e.target.value })
                  setSaved(false)
                }}
              />
            </Field>
            <label className="flex h-10 cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-primary"
                disabled={!canEdit}
                checked={definition.isSample}
                onChange={(e) => {
                  setDefinition({ ...definition, isSample: e.target.checked })
                  setSaved(false)
                }}
              />
              作为公开样例
            </label>
          </div>
          <div className="grid gap-4 xl:grid-cols-2">
            {(['input', 'answer'] as const).map((part) => {
              const file = copy.tree.entries.find((e) => e.id === definition[part].entry),
                label = part === 'input' ? '输入' : '期望输出'
              return (
                <section
                  key={part}
                  className="min-w-0 overflow-hidden rounded-xl border bg-background"
                >
                  <header className="flex items-center justify-between gap-2 border-b bg-muted/30 px-3 py-2">
                    <span className="text-sm font-medium">{label}</span>
                    <div className="flex gap-1">
                      {file && (
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          aria-label={`下载${label}`}
                          onClick={() => void download(file)}
                        >
                          <Download className="size-4" />
                        </Button>
                      )}
                      {canEdit && (
                        <Button
                          type="button"
                          size="icon"
                          variant="ghost"
                          aria-label={`替换${label}文件`}
                          onClick={() => inputs.current[part]?.click()}
                        >
                          <Upload className="size-4" />
                        </Button>
                      )}
                    </div>
                  </header>
                  <input
                    type="file"
                    className="hidden"
                    ref={(el) => {
                      inputs.current[part] = el
                    }}
                    onChange={(e) => {
                      const picked = e.target.files?.[0]
                      if (picked) {
                        setFiles((current) => ({ ...current, [part]: picked }))
                        setSaved(false)
                      }
                      e.target.value = ''
                    }}
                  />
                  {files[part] ? (
                    <div className="space-y-2 p-5 text-sm">
                      <p>将替换为 {files[part]!.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {formatFileSize(files[part]!.size)}
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setFiles((current) => ({ ...current, [part]: undefined }))}
                      >
                        取消替换
                      </Button>
                    </div>
                  ) : text[part] !== undefined ? (
                    <Textarea
                      aria-label={`编辑${label}`}
                      disabled={!canEdit}
                      value={text[part]}
                      className="h-64 resize-none rounded-none border-0 font-mono text-xs focus-visible:ring-inset"
                      spellCheck={false}
                      onChange={(e) => {
                        setText((current) => ({ ...current, [part]: e.target.value }))
                        setSaved(false)
                      }}
                    />
                  ) : (
                    <div className="flex min-h-40 items-center justify-center p-5 text-center text-xs leading-6 text-muted-foreground">
                      {file
                        ? `文件大小 ${formatFileSize(file.blob.bytes)}，请下载查看或上传新文件。`
                        : definition[part].kind === 'file'
                          ? '尚未配置文件，点击右上角上传。'
                          : '检查时由程序生成，完成后可在检查报告中查看。'}
                    </div>
                  )}
                </section>
              )
            })}
          </div>
        </fieldset>
      )}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
        <Button size="sm" variant="ghost" disabled={busy || dirty} onClick={onAdvanced}>
          生成方式、分组与限制
        </Button>
        {canEdit && (
          <SaveButton
            loading={busy}
            saved={saved && !dirty}
            disabled={loading || (!dirty && !saved) || !definition?.name.trim()}
            onClick={() => void save()}
          >
            保存测试点
          </SaveButton>
        )}
      </div>
    </div>
  )
}
