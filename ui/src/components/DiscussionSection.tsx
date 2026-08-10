import { useEffect, useState } from 'react'
import { MessageSquare, Reply, Trash2 } from 'lucide-react'
import type { DtoDiscussionResponse as Discussion } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import MdRenderer from '@/components/MdRenderer'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/input'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { useToast } from '@/components/ui/toast'
import { apiError, formatRelative } from '@/lib/format'

type DiscussionSectionProps = {
  fetchPosts: () => Promise<Discussion[]>
  createPost: (content: string, parentId?: number) => Promise<void>
  onDelete: (postId: number) => Promise<void>
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
  onDelete,
}: DiscussionSectionProps) {
  const { user } = useAuth()
  const toast = useToast()
  const [posts, setPosts] = useState<Discussion[]>([])
  const [loading, setLoading] = useState(true)
  const [content, setContent] = useState('')
  const [replyTo, setReplyTo] = useState<Discussion | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function load() {
    try {
      setPosts(await fetchPosts())
    } catch (error) {
      toast.error(apiError(error, '讨论加载失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleSubmit() {
    if (!content.trim()) {
      toast.warning('请输入内容')
      return
    }
    setSubmitting(true)
    try {
      await createPost(content.trim(), replyTo?.id)
      setContent('')
      setReplyTo(null)
      await load()
    } catch (error) {
      toast.error(apiError(error, '发表失败'))
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDelete(postId: number) {
    try {
      await onDelete(postId)
      await load()
      toast.success('已删除')
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
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
      {user ? (
        <div className="flex flex-col gap-2">
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
            rows={3}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            placeholder="写下你的想法,支持 Markdown 与 LaTeX"
          />
          <div className="flex justify-end">
            <Button size="sm" loading={submitting} onClick={handleSubmit}>
              发表
            </Button>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">登录后可以参与讨论。</p>
      )}

      {threads.length === 0 ? (
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
              canModerate={(post) => Boolean(user && (user.role === 'admin' || user.id === post.authorId))}
              canReply={Boolean(user)}
              onReply={setReplyTo}
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
  canReply,
  onReply,
  onDelete,
}: {
  node: ThreadNode
  depth: number
  canModerate: (post: Discussion) => boolean
  canReply: boolean
  onReply: (post: Discussion) => void
  onDelete: (postId: number) => void
}) {
  return (
    <div className={depth > 0 ? 'border-l border-border pl-4' : undefined}>
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{node.authorName}</span>
          <span title={node.createdAt}>{formatRelative(node.createdAt)}</span>
          <div className="ml-auto flex items-center gap-1">
            {canReply ? (
              <Button variant="ghost" size="icon-sm" onClick={() => onReply(node)} aria-label="回复">
                <Reply />
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
        <MdRenderer content={node.contentMd} className="text-sm" />
      </div>

      {node.replies.length > 0 ? (
        <div className="mt-3 flex flex-col gap-3">
          {node.replies.map((reply) => (
            <PostNode
              key={reply.id}
              node={reply}
              depth={depth + 1}
              canModerate={canModerate}
              canReply={canReply}
              onReply={onReply}
              onDelete={onDelete}
            />
          ))}
        </div>
      ) : null}
    </div>
  )
}
