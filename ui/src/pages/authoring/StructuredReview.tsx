import { useEffect, useState, type ReactNode } from 'react'
import type { DomainContentComparison, DomainReviewItem } from '@/generated/api/model'
import { Button } from '@/components/ui/button'
import { Code2, FileText, Database, Settings2, Image, Wand2, ArrowRight, Check } from 'lucide-react'
import { materialFieldNames } from './material-diff'
import { roleNames } from '@/lib/authoring-materials'
import MdRenderer, { markdownImages } from '@/components/MdRenderer'
import ReviewTextDiff from './ReviewTextDiff'

const categories = [
  { id: 'statement', name: '题面', icon: FileText },
  { id: 'program', name: '程序', icon: Code2 },
  { id: 'generation', name: '生成方案', icon: Wand2 },
  { id: 'test', name: '测试数据', icon: Database },
  { id: 'metadata', name: '评测规则', icon: Settings2 },
  { id: 'other', name: '其他材料', icon: Image },
]
function category(item: DomainReviewItem) {
  return categories.some((c) => c.id === item.kind) ? item.kind : 'other'
}
function label(item: DomainReviewItem) {
  return item.kind === 'statement'
    ? item.label.replace(/ · zh$/, ' · 中文').replace(/ · en$/, ' · English')
    : item.label
}
function friendly(key: string, value: string) {
  if (!value) return '未设置'
  if (key === 'role') return roleNames[value] ?? value
  if (value === 'true') return '是'
  if (value === 'false') return '否'
  if (key === 'expectedVerdicts')
    return value
      .replace(/Accepted/g, '通过')
      .replace(/Wrong Answer/g, '答案错误')
      .replace(/Time Limit Exceeded/g, '超时')
  return value
}
function ReviewValue({
  item,
  fieldKey,
  value,
}: {
  item: DomainReviewItem
  fieldKey: string
  value: string
}) {
  if (item.kind === 'statement' && value && !item.truncated)
    return (
      <MdRenderer
        content={value}
        className="text-sm [&_p]:leading-6"
        media={{
          images: Object.fromEntries(markdownImages(value).map((src) => [src, ''])),
          links: {},
        }}
      />
    )
  return (
    <pre className="whitespace-pre-wrap break-words font-sans text-xs leading-5">
      {friendly(fieldKey, value)}
    </pre>
  )
}
export default function StructuredReview({
  comparison,
  renderDetails,
}: {
  comparison?: DomainContentComparison
  renderDetails: (item: DomainReviewItem) => ReactNode
}) {
  const [selected, setSelected] = useState(''),
    [filter, setFilter] = useState('all'),
    [details, setDetails] = useState(false),
    [rendered, setRendered] = useState(false),
    [page, setPage] = useState(0)
  const items = [...(comparison?.review ?? [])].sort(
      (a, b) =>
        categories.findIndex((c) => c.id === category(a)) -
        categories.findIndex((c) => c.id === category(b)),
    ),
    filtered = items.filter((item) => filter === 'all' || category(item) === filter),
    item = filtered.find((i) => i.id === selected) ?? filtered[0]
  useEffect(() => {
    setDetails(item?.kind === 'resource')
    setRendered(false)
    setPage(0)
  }, [item?.id, comparison])
  if (!comparison)
    return <p className="py-12 text-center text-sm text-muted-foreground">正在整理业务更改…</p>
  if (!items.length)
    return (
      <div className="rounded-xl border border-dashed py-14 text-center">
        <Check className="mx-auto mb-3 size-7 text-muted-foreground" />
        <p className="text-sm">当前没有待审阅的更改</p>
      </div>
    )
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant={filter === 'all' ? 'secondary' : 'outline'}
          onClick={() => setFilter('all')}
        >
          全部 · {items.length}
        </Button>
        {categories.map((c) => {
          const count = items.filter((i) => category(i) === c.id).length
          return count ? (
            <Button
              key={c.id}
              size="sm"
              variant={filter === c.id ? 'secondary' : 'outline'}
              onClick={() => setFilter(c.id)}
            >
              <c.icon className="size-3.5" />
              {c.name} · {count}
            </Button>
          ) : null
        })}
      </div>
      <div className="grid min-w-0 gap-4 lg:grid-cols-[210px_minmax(0,1fr)]">
        <nav
          aria-label="更改的业务内容"
          className="max-h-[65dvh] space-y-1 overflow-y-auto rounded-xl border bg-card p-2"
        >
          {filtered.map((i) => (
            <button
              key={i.id}
              type="button"
              onClick={() => setSelected(i.id)}
              aria-current={item?.id === i.id ? 'page' : undefined}
              className={`flex w-full items-start gap-2 rounded-lg border p-3 text-left ${item?.id === i.id ? 'border-primary/30 bg-primary/10' : 'border-transparent hover:bg-muted/40'}`}
            >
              <span className="mt-0.5 text-xs font-medium text-primary">
                {i.change === 'added' ? '+' : i.change === 'deleted' ? '−' : '~'}
              </span>
              <span className="min-w-0">
                <span className="block break-words text-sm font-medium">{label(i)}</span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  {categories.find((c) => c.id === category(i))?.name} · {i.fields.length} 项调整
                </span>
              </span>
            </button>
          ))}
        </nav>
        {item && (
          <section className="min-w-0 overflow-hidden rounded-xl border bg-card">
            <header className="flex flex-wrap items-center justify-between gap-3 border-b p-4">
              <div>
                <h3 className="font-semibold">{label(item)}</h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  {item.change === 'added'
                    ? '本次新增'
                    : item.change === 'deleted'
                      ? '本次移除'
                      : '核对修改前后的内容'}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                {item.kind === 'statement' && !details && (
                  <Button size="sm" variant="outline" onClick={() => setRendered(!rendered)}>
                    {rendered ? '逐行差异' : '排版预览'}
                  </Button>
                )}
                {(['program', 'test', 'asset', 'resource', 'statement'].includes(item.kind) ||
                  item.truncated) && (
                  <Button size="sm" variant="outline" onClick={() => setDetails(!details)}>
                    {details
                      ? '返回结构化审阅'
                      : item.kind === 'program'
                        ? '查看代码差异'
                        : '查看内容差异'}
                  </Button>
                )}
              </div>
            </header>
            {details ? (
              <div className="space-y-5 p-4">{renderDetails(item)}</div>
            ) : (
              <div className="divide-y">
                {item.fields.slice(page * 12, page * 12 + 12).map((field, index) => (
                  <div key={field.key + index} className="p-4">
                    <h4 className="mb-3 text-sm font-medium">
                      {materialFieldNames[field.key] ?? field.key}
                    </h4>
                    {item.kind === 'statement' && !rendered ? (
                      <ReviewTextDiff before={field.before} after={field.after} />
                    ) : (
                      <div className="grid min-w-0 items-start gap-3 md:grid-cols-[minmax(0,1fr)_20px_minmax(0,1fr)]">
                        <div className="min-w-0 rounded-lg border border-red-300/60 bg-red-50/70 p-3 dark:border-red-500/25 dark:bg-red-500/10">
                          <p className="mb-2 text-[11px] text-red-800 dark:text-red-300">
                            − 修改前
                          </p>
                          <div className="max-h-72 overflow-auto">
                            <ReviewValue item={item} fieldKey={field.key} value={field.before} />
                          </div>
                        </div>
                        <ArrowRight className="hidden size-4 self-center text-muted-foreground md:block" />
                        <div className="min-w-0 rounded-lg border border-green-300/60 bg-green-50/70 p-3 dark:border-green-500/25 dark:bg-green-500/10">
                          <p className="mb-2 text-[11px] text-green-800 dark:text-green-300">
                            + 修改后
                          </p>
                          <div className="max-h-72 overflow-auto">
                            <ReviewValue item={item} fieldKey={field.key} value={field.after} />
                          </div>
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
            {item.fields.length > 12 && !details && (
              <div className="flex items-center justify-between border-t p-3 text-xs text-muted-foreground">
                <span>
                  第 {page + 1} / {Math.ceil(item.fields.length / 12)} 页
                </span>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={!page}
                    onClick={() => setPage(page - 1)}
                  >
                    上一页
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={(page + 1) * 12 >= item.fields.length}
                    onClick={() => setPage(page + 1)}
                  >
                    下一页
                  </Button>
                </div>
              </div>
            )}
            {item.truncated && (
              <p className="border-t p-3 text-xs text-muted-foreground">
                较长内容已截取摘要，可查看完整内容差异。
              </p>
            )}
          </section>
        )}
      </div>
    </div>
  )
}
