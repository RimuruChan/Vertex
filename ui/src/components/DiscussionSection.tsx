import { useEffect, useRef, useState } from 'react'
import { MessageSquare, Pencil, Reply, Trash2 } from 'lucide-react'
import type {
  DtoDiscussionResponse as Discussion,
  DtoDiscussionThreadResponse,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import MdRenderer from '@/components/MdRenderer'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Textarea } from '@/components/ui/input'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { useToast } from '@/components/ui/toast'
import { apiError, formatRelative } from '@/lib/format'

type DiscussionSectionProps = {
  fetchPosts: () => Promise<DtoDiscussionThreadResponse>
  createPost: (content: string, parentId?: number) => Promise<void>
  onUpdate?: (postId: number, content: string) => Promise<void>
  onDelete: (postId: number) => Promise<void>
  canReply?: boolean
  reloadKey?: string | number
}

type ThreadNode = Discussion & { replies: ThreadNode[] }

/** Flat post list → nested threads, preserving server order within a level. */
function buildThreads(posts: Discussion[]): ThreadNode[] {
  const nodes = new Map<number, ThreadNode>()
  for (const post of posts) nodes.set(post.id, { ...post, replies: [] })

  const roots: ThreadNode[] = []
  for (const post of posts) {
    const node = nodes.get(post.id)!
    const parent = post.parentId ? nodes.get(post.parentId) : undefined
    if (parent) parent.replies.push(node)
    else roots.push(node)
  }
  return roots
}

export default function DiscussionSection({
  fetchPosts,
  createPost,
  onUpdate,
  onDelete,
  canReply = true,
  reloadKey,
}: DiscussionSectionProps) {
  const { user } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()
  const [posts, setPosts] = useState<Discussion[]>([])
  const [loading, setLoading] = useState(true)
  const [content, setContent] = useState('')
  const [replyTo, setReplyTo] = useState<Discussion | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const [canPost, setCanPost] = useState(false)
  const composerRef = useRef<HTMLTextAreaElement>(null)
  const requestSequence = useRef(0)
  const identityKey = `${reloadKey ?? ''}:${user?.id ?? ''}:${user?.role ?? ''}`
  const activeIdentity = useRef(identityKey)
  activeIdentity.current = identityKey

  async function load() {
    const sequence = ++requestSequence.current
    setLoadError(false)
    try {
      const result = await fetchPosts()
      if (sequence !== requestSequence.current) return
      setPosts(result.items)
      setCanPost(result.canPost)
    } catch {
      if (sequence !== requestSequence.current) return
      setLoadError(true)
      setCanPost(false)
    } finally {
      if (sequence === requestSequence.current) setLoading(false)
    }
  }

  useEffect(() => {
    setLoading(true)
    setPosts([])
    setContent('')
    setReplyTo(null)
    setCanPost(false)
    setSubmitting(false)
    void load()
    return () => {
      requestSequence.current += 1
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadKey, identityKey])

  useEffect(() => {
    if (!replyTo) return
    composerRef.current?.scrollIntoView({
      behavior: 'smooth',
      block: 'center',
    })
    composerRef.current?.focus()
  }, [replyTo])

  async function handleSubmit() {
    if (submitting || !canPost || loadError) return
    if (!content.trim()) {
      toast.warning('请输入内容')
      return
    }
    const key = identityKey
    setSubmitting(true)
    try {
      await createPost(content.trim(), replyTo?.id)
      if (activeIdentity.current !== key) return
      setContent('')
      setReplyTo(null)
      await load()
    } catch (error) {
      if (activeIdentity.current === key) toast.error(apiError(error, '发表失败'))
    } finally {
      if (activeIdentity.current === key) setSubmitting(false)
    }
  }

  async function handleDelete(postId: number) {
    if (!posts.find((p) => p.id === postId)?.permissions.delete) return
    const key = identityKey
    const accepted = await confirm({
      title: '删除这条讨论？',
      description: '这会同时删除它下面的全部回复，且无法恢复。',
      confirmLabel: '删除讨论',
      destructive: true,
    })
    if (!accepted || activeIdentity.current !== key) return
    try {
      await onDelete(postId)
      if (activeIdentity.current !== key) return
      await load()
      toast.success('已删除')
    } catch (error) {
      if (activeIdentity.current === key) toast.error(apiError(error, '删除失败'))
    }
  }

  async function handleUpdate(postId: number, nextContent: string) {
    if (!onUpdate || !posts.find((p) => p.id === postId)?.permissions.edit) return
    const key = identityKey
    try {
      await onUpdate(postId, nextContent)
      if (activeIdentity.current !== key) return
      await load()
      toast.success('讨论已更新')
    } catch (error) {
      if (activeIdentity.current === key) toast.error(apiError(error, '更新失败'))
      throw error
    }
  }

  if (loading) {
    return (
      <div className="flex flex-col gap-3">
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    )
  }

  const threads = buildThreads(posts)

  return (
    <div className="flex flex-col gap-5">
      {canPost && !loadError ? (
        <div className="flex flex-col gap-3 rounded-lg bg-muted/40 p-4">
          <p className="text-sm font-medium">{replyTo ? '继续这段讨论' : '说说你的思路'}</p>
          {replyTo ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Reply className="size-3.5" />
              回复 <span className="font-medium text-foreground">{replyTo.authorName}</span>
              <button
                type="button"
                className="text-primary hover:underline"
                onClick={() => setReplyTo(null)}
              >
                取消
              </button>
            </div>
          ) : null}
          <Textarea
            ref={composerRef}
            aria-label={replyTo ? `回复 ${replyTo.authorName}` : '讨论内容'}
            rows={3}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            placeholder="写下你的想法，支持 Markdown 与 LaTeX"
          />
          <div className="flex justify-end">
            <Button size="sm" loading={submitting} onClick={handleSubmit}>
              发表
            </Button>
          </div>
        </div>
      ) : !loadError ? (
        <p className="text-sm text-muted-foreground">
          {user ? '当前视角只读，无法发表新讨论。' : '登录后可以参与讨论。'}
        </p>
      ) : null}

      {loadError ? (
        <div className="border-y border-border py-8 text-center">
          <p className="text-sm text-muted-foreground">讨论暂时加载失败。</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => void load()}>
            重试
          </Button>
        </div>
      ) : threads.length === 0 ? (
        <EmptyState
          icon={<MessageSquare />}
          title="还没有讨论"
          description="有思路或疑问?来开第一个话题。"
        />
      ) : (
        <div className="flex flex-col gap-4">
          {threads.map((thread) => (
            <PostNode
              key={thread.id}
              node={thread}
              depth={0}
              canModerate={(post) => post.permissions.delete}
              canEdit={(post) => Boolean(post.permissions.edit && onUpdate)}
              canReply={canPost && canReply}
              onReply={setReplyTo}
              onUpdate={handleUpdate}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function PostNode({
  node,
  depth,
  canModerate,
  canEdit,
  canReply,
  onReply,
  onUpdate,
  onDelete,
}: {
  node: ThreadNode
  depth: number
  canModerate: (post: Discussion) => boolean
  canEdit: (post: Discussion) => boolean
  canReply: boolean
  onReply: (post: Discussion) => void
  onUpdate: (postId: number, content: string) => Promise<void>
  onDelete: (postId: number) => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(node.contentMd)
  const [saving, setSaving] = useState(false)

  async function saveEdit() {
    const content = draft.trim()
    if (!content || content === node.contentMd) {
      setEditing(false)
      setDraft(node.contentMd)
      return
    }
    setSaving(true)
    try {
      await onUpdate(node.id, content)
      setEditing(false)
    } catch {
      // The parent owns the API contract. Keep the draft open so it is not lost.
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className={
        depth > 0
          ? depth <= 2
            ? 'border-l border-border pl-4'
            : 'border-l border-border/60 pl-2'
          : undefined
      }
    >
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{node.authorName}</span>
          <span title={node.createdAt}>{formatRelative(node.createdAt)}</span>
          {node.edited ? <span>已编辑</span> : null}
          <div className="ml-auto flex items-center gap-1">
            {canReply ? (
              <Button variant="ghost" size="sm" onClick={() => onReply(node)}>
                <Reply />
                回复
              </Button>
            ) : null}
            {canEdit(node) ? (
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setEditing((value) => !value)}
                aria-label="编辑讨论"
              >
                <Pencil />
              </Button>
            ) : null}
            {canModerate(node) ? (
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onDelete(node.id)}
                aria-label="删除"
                className="hover:text-destructive"
              >
                <Trash2 />
              </Button>
            ) : null}
          </div>
        </div>
        {editing ? (
          <div className="flex flex-col gap-2">
            <Textarea
              aria-label="编辑讨论内容"
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              rows={4}
            />
            <div className="flex justify-end gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setEditing(false)
                  setDraft(node.contentMd)
                }}
              >
                取消
              </Button>
              <Button size="sm" loading={saving} onClick={() => void saveEdit()}>
                保存
              </Button>
            </div>
          </div>
        ) : (
          <MdRenderer content={node.contentMd} className="text-sm" />
        )}
      </div>

      {node.replies.length > 0 ? (
        <div className="mt-3 flex flex-col gap-3">
          {node.replies.map((reply) => (
            <PostNode
              key={reply.id}
              node={reply}
              depth={depth + 1}
              canModerate={canModerate}
              canEdit={canEdit}
              canReply={canReply}
              onReply={onReply}
              onUpdate={onUpdate}
              onDelete={onDelete}
            />
          ))}
        </div>
      ) : null}
    </div>
  )
}
