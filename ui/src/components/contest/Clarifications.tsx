import { useCallback, useEffect, useRef, useState } from 'react'
import { Megaphone, MessageSquarePlus, Send } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoClarificationResponse as Clarification,
  DtoContestProblemResponse as ContestProblem,
} from '@/generated/api/model'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ContestPanel as Card } from '@/components/contest/ContestPageLayout'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import MdRenderer from '@/components/MdRenderer'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime } from '@/lib/format'

const NONE = 'none'
const POLL_MS = 15000

/**
 * The contest question channel. Contestants open threads and read
 * announcements; the jury answers threads and broadcasts to everyone.
 */
export default function Clarifications({
  contestId,
  problems,
  isJury,
  canAsk,
  readAll = false,
}: {
  contestId: string
  problems: ContestProblem[]
  isJury: boolean
  canAsk: boolean
  readAll?: boolean
}) {
  const {
    getApiContestsIdClarifications: listClarifications,
    postApiContestsIdClarifications: askClarification,
    postApiContestsIdClarificationsReply: replyClarification,
  } = useDomainAPI()
  const toast = useToast()
  const [items, setItems] = useState<Clarification[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const activeRequest = useRef<AbortController | null>(null)
  const [sending, setSending] = useState(false)
  const [problemId, setProblemId] = useState(NONE)
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [replyTo, setReplyTo] = useState<Clarification | null>(null)
  const [replyBody, setReplyBody] = useState('')

  const load = useCallback(async () => {
    activeRequest.current?.abort()
    const controller = new AbortController()
    activeRequest.current = controller
    try {
      const result = await listClarifications(contestId, { signal: controller.signal })
      if (controller.signal.aborted) return
      setItems(result.items)
      setLoadError(null)
    } catch (error) {
      if (!controller.signal.aborted) setLoadError(apiError(error, '答疑加载失败'))
    } finally {
      if (!controller.signal.aborted) setLoading(false)
      if (activeRequest.current === controller) activeRequest.current = null
    }
  }, [contestId])

  useEffect(() => {
    void load()
    const timer = window.setInterval(() => {
      if (!activeRequest.current) void load()
    }, POLL_MS)
    return () => {
      window.clearInterval(timer)
      activeRequest.current?.abort()
    }
  }, [load])

  async function handleAsk() {
    if (!body.trim()) {
      toast.warning('请填写问题内容')
      return
    }
    setSending(true)
    try {
      await askClarification(contestId, {
        problemId: problemId === NONE ? undefined : problemId,
        subject: subject.trim() || undefined,
        body,
      })
      toast.success('已提交,等待裁判回复')
      setBody('')
      setSubject('')
      await load()
    } catch (error) {
      toast.error(apiError(error, '提交失败'))
    } finally {
      setSending(false)
    }
  }

  async function handleReply(parent: Clarification | null) {
    const text = parent ? replyBody : body
    if (!text.trim()) {
      toast.warning('请填写内容')
      return
    }
    setSending(true)
    try {
      await replyClarification(contestId, {
        parentId: parent?.id,
        problemId: parent ? undefined : problemId === NONE ? undefined : problemId,
        subject: parent ? undefined : subject.trim() || undefined,
        body: text,
      })
      toast.success(parent ? '已回复' : '公告已发布')
      if (parent) {
        setReplyTo(null)
        setReplyBody('')
      } else {
        setBody('')
        setSubject('')
      }
      await load()
    } catch (error) {
      toast.error(apiError(error, '发送失败'))
    } finally {
      setSending(false)
    }
  }

  return (
    <div
      className={
        isJury || canAsk
          ? 'grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]'
          : 'grid gap-5'
      }
    >
      <Card className="flex min-w-0 flex-col gap-4 p-5">
        <p className="text-sm font-medium">{isJury || readAll ? '全部答疑' : '我的提问与公告'}</p>
        {loadError ? (
          <div
            role="alert"
            className="flex items-center justify-between gap-3 rounded-lg border border-destructive/30 p-3 text-sm text-destructive"
          >
            <span>{loadError}</span>
            <Button size="sm" variant="outline" onClick={() => void load()}>
              重新加载
            </Button>
          </div>
        ) : null}
        {loading ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 3 }, (_, index) => (
              <Skeleton key={index} className="h-16 w-full" />
            ))}
          </div>
        ) : items.length === 0 ? (
          loadError ? null : (
            <EmptyState
              title="还没有答疑"
              description={
                isJury
                  ? '选手提问会出现在这里。'
                  : canAsk
                    ? '有疑问可以向裁判提问。'
                    : readAll
                      ? '当前为只读视角，可查看比赛提问与公告。'
                      : '比赛公告和已有提问会出现在这里。'
              }
            />
          )
        ) : (
          <div className="flex flex-col gap-3">
            {items.map((item) => (
              <article
                key={item.id}
                className="min-w-0 rounded-xl border border-border bg-background/40 p-4"
              >
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  {item.announce ? (
                    <Badge variant="warning">
                      <Megaphone className="size-3" />
                      公告
                    </Badge>
                  ) : item.fromJury ? (
                    <Badge variant="secondary">裁判</Badge>
                  ) : (
                    <Badge variant="outline">{item.authorName || '选手'}</Badge>
                  )}
                  {item.problemName ? <span>题目:{item.problemName}</span> : null}
                  <span>{formatDateTime(item.createdAt)}</span>
                  {!item.fromJury && !item.answered ? (
                    <Badge variant="destructive">待回复</Badge>
                  ) : null}
                </div>
                {item.subject ? <p className="mt-1 text-sm font-medium">{item.subject}</p> : null}
                <MdRenderer
                  className="mt-3 min-w-0 text-sm [&>:first-child]:mt-0 [&>:last-child]:mb-0"
                  content={item.body}
                />

                {item.replies.length > 0 ? (
                  <div className="mt-4 flex flex-col gap-4 border-l-2 border-primary/20 pl-4">
                    {item.replies.map((reply) => (
                      <div key={reply.id}>
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          <Badge variant="secondary">裁判</Badge>
                          <span>{formatDateTime(reply.createdAt)}</span>
                        </div>
                        <MdRenderer
                          className="mt-2 min-w-0 text-sm [&>:first-child]:mt-0 [&>:last-child]:mb-0"
                          content={reply.body}
                        />
                      </div>
                    ))}
                  </div>
                ) : null}

                {isJury && !item.fromJury ? (
                  replyTo?.id === item.id ? (
                    <div className="mt-2 flex flex-col gap-2">
                      <MarkdownComposer
                        id={`clar-reply-${item.id}`}
                        label="回复内容"
                        value={replyBody}
                        onChange={setReplyBody}
                        disabled={sending}
                        placeholder="回复内容(只有提问者可见)"
                      />
                      <div className="flex gap-2">
                        <Button size="sm" loading={sending} onClick={() => handleReply(item)}>
                          <Send />
                          发送回复
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setReplyTo(null)}>
                          取消
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <Button
                      size="sm"
                      variant="outline"
                      className="mt-2"
                      onClick={() => {
                        setReplyTo(item)
                        setReplyBody('')
                      }}
                    >
                      回复
                    </Button>
                  )
                ) : null}
              </article>
            ))}
          </div>
        )}
      </Card>

      {isJury || canAsk ? (
        <Card className="flex min-w-0 flex-col gap-4 p-5">
          <p className="text-sm font-medium">{isJury ? '发布公告' : '向裁判提问'}</p>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="clar-problem">关联题目</Label>
            <Select value={problemId} onValueChange={setProblemId}>
              <SelectTrigger id="clar-problem">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>不指定</SelectItem>
                {problems.map((problem) => (
                  <SelectItem key={problem.problemId} value={problem.problemId}>
                    {problem.label} — {problem.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="clar-subject">标题</Label>
            <Input
              id="clar-subject"
              value={subject}
              onChange={(event) => setSubject(event.target.value)}
              placeholder="可留空"
            />
          </div>
          <MarkdownComposer
            id="clar-body"
            label="内容"
            value={body}
            onChange={setBody}
            disabled={sending}
            placeholder={isJury ? '发给全场的公告' : '描述你的疑问'}
          />
          <Button loading={sending} onClick={() => (isJury ? handleReply(null) : handleAsk())}>
            {isJury ? <Megaphone /> : <MessageSquarePlus />}
            {isJury ? '发布公告' : '提交提问'}
          </Button>
        </Card>
      ) : null}
    </div>
  )
}

function MarkdownComposer({
  id,
  label,
  value,
  onChange,
  placeholder,
  disabled,
}: {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  placeholder: string
  disabled: boolean
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Tabs defaultValue="write" className="min-w-0 rounded-lg border border-border">
        <TabsList aria-label={`${label}编辑模式`} className="h-9 px-3">
          <TabsTrigger value="write">编写</TabsTrigger>
          <TabsTrigger value="preview">预览</TabsTrigger>
        </TabsList>
        <TabsContent value="write" className="mt-0">
          <Textarea
            id={id}
            rows={6}
            value={value}
            disabled={disabled}
            onChange={(event) => onChange(event.target.value)}
            placeholder={placeholder}
            className="rounded-t-none border-0 shadow-none"
          />
        </TabsContent>
        <TabsContent value="preview" className="mt-0 min-h-36 overflow-x-auto p-3">
          {value.trim() ? (
            <MdRenderer content={value} className="text-sm [&>:first-child]:mt-0" />
          ) : (
            <p className="text-sm text-muted-foreground">暂无预览内容</p>
          )}
        </TabsContent>
      </Tabs>
      <p className="text-xs text-muted-foreground">支持 Markdown：列表、代码块、链接与数学公式。</p>
    </div>
  )
}
