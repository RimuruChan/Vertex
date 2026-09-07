import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import { ArrowLeft, Lock, MessageSquare, Pencil, Save, ThumbsUp, Trash2 } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type { DtoEditorialResponse as Editorial } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { useCanonicalResourcePath } from '@/hooks/useCanonicalPath'
import DiscussionSection from '@/components/DiscussionSection'
import MdRenderer from '@/components/MdRenderer'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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

export default function EditorialDetailPage() {
  const {
    deleteApiDiscussionsPostId: deleteDiscussion,
    deleteApiEditorialsId: deleteEditorial,
    getApiEditorialsId: getEditorial,
    getApiEditorialsIdDiscussions: listDiscussions,
    postApiEditorialsIdDiscussions: createDiscussion,
    postApiEditorialsIdVote: voteEditorial,
    putApiDiscussionsPostId: updateDiscussion,
    putApiEditorialsId: updateEditorial,
  } = useDomainAPI()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user, ready } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()
  const [editorial, setEditorial] = useState<Editorial | null>(null)
  useCanonicalResourcePath('editorials', id, editorial)
  const [loading, setLoading] = useState(true)
  const [loadedIdentity, setLoadedIdentity] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [voting, setVoting] = useState(false)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [draftTitle, setDraftTitle] = useState('')
  const [draftContent, setDraftContent] = useState('')
  const [draftVisibility, setDraftVisibility] = useState<'public' | 'private'>('public')
  const [draftStatus, setDraftStatus] = useState<'draft' | 'published'>('published')
  const [draftSolvedOnly, setDraftSolvedOnly] = useState(false)
  const identityKey = `${id ?? ''}:${user?.id ?? ''}:${user?.role ?? ''}`
  const activeIdentity = useRef(identityKey)
  activeIdentity.current = identityKey

  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!id || !ready) return
      setLoading(true)
      setError(null)
      setEditing(false)
      setVoting(false)
      setSaving(false)
      try {
        const result = await getEditorial(id, { signal })
        if (!signal?.aborted) setEditorial(result)
      } catch (caught) {
        if (signal?.aborted) return
        setEditorial(null)
        setError(apiError(caught, '题解加载失败'))
      } finally {
        if (!signal?.aborted) {
          setLoadedIdentity(identityKey)
          setLoading(false)
        }
      }
    },
    [id, ready, user?.id, user?.role, identityKey],
  )

  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  useEffect(() => {
    if (editorial) document.title = `${editorial.title} · Vertex`
  }, [editorial])

  async function handleVote() {
    if (!editorial || !user) {
      toast.warning('登录后可以赞同题解')
      return
    }
    if (voting || !editorial.permissions.vote) return
    const key = identityKey
    setVoting(true)
    try {
      const result = await voteEditorial(editorial.id, {
        up: !editorial.voted,
      })
      if (activeIdentity.current !== key) return
      setEditorial({
        ...editorial,
        voted: !editorial.voted,
        voteCount: result.voteCount,
      })
    } catch (caught) {
      if (activeIdentity.current === key) toast.error(apiError(caught, '操作失败'))
    } finally {
      if (activeIdentity.current === key) setVoting(false)
    }
  }

  async function handleDelete() {
    if (!editorial?.permissions.delete) return
    const key = identityKey
    const accepted = await confirm({
      title: '删除这篇题解？',
      description: '题解正文及其讨论会被永久删除。',
      confirmLabel: '删除题解',
      destructive: true,
    })
    if (!accepted || activeIdentity.current !== key) return
    try {
      await deleteEditorial(editorial.id)
      if (activeIdentity.current !== key) return
      toast.success('题解已删除')
      navigate('/editorials')
    } catch (caught) {
      if (activeIdentity.current === key) toast.error(apiError(caught, '删除失败'))
    }
  }

  function beginEditing() {
    if (!editorial?.permissions.edit) return
    setDraftTitle(editorial.title)
    setDraftContent(editorial.contentMd)
    setDraftVisibility(editorial.visibility)
    setDraftStatus(editorial.status)
    setDraftSolvedOnly(editorial.solvedOnly)
    setEditing(true)
  }

  async function cancelEditing() {
    if (!editorial) return
    if (
      editDirty &&
      !(await confirm({
        title: '放弃修改？',
        description: '尚未保存的题解内容会丢失。',
        confirmLabel: '放弃修改',
        destructive: true,
      }))
    )
      return
    setEditing(false)
  }

  async function handleSave() {
    if (!editorial?.permissions.edit || saving) return
    const key = identityKey
    if (!draftTitle.trim() || !draftContent.trim()) {
      toast.warning('请填写标题与正文')
      return
    }
    setSaving(true)
    try {
      const updated = await updateEditorial(editorial.id, {
        title: draftTitle.trim(),
        contentMd: draftContent.trim(),
        visibility: draftVisibility,
        status: draftStatus,
        solvedOnly: draftSolvedOnly,
      })
      if (activeIdentity.current !== key) return
      setEditorial(updated)
      setEditing(false)
      toast.success('题解已保存')
    } catch (caught) {
      if (activeIdentity.current === key) toast.error(apiError(caught, '保存失败'))
    } finally {
      if (activeIdentity.current === key) setSaving(false)
    }
  }

  const editDirty = Boolean(
    editorial &&
    editing &&
    (draftTitle !== editorial.title ||
      draftContent !== editorial.contentMd ||
      draftVisibility !== editorial.visibility ||
      draftStatus !== editorial.status ||
      draftSolvedOnly !== editorial.solvedOnly),
  )

  useEffect(() => {
    if (!editDirty) return
    const warnBeforeLeaving = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeLeaving)
    return () => window.removeEventListener('beforeunload', warnBeforeLeaving)
  }, [editDirty])

  if (loading || loadedIdentity !== identityKey) {
    return (
      <div className="mx-auto w-full max-w-5xl px-4 py-10 sm:px-6">
        <Skeleton className="mb-5 h-8 w-2/3" />
        <Skeleton className="h-80 w-full" />
      </div>
    )
  }

  if (!editorial) {
    return (
      <EmptyState
        title="无法打开题解"
        description={error || '题解可能已被删除，或者你没有查看权限。'}
        action={
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => void load()}>
              重试
            </Button>
            <Button variant="ghost" asChild>
              <Link to="/editorials">返回题解</Link>
            </Button>
          </div>
        }
      />
    )
  }

  return (
    <div className="mx-auto grid w-full max-w-6xl gap-10 px-4 py-8 sm:px-6 lg:grid-cols-[minmax(0,1fr)_260px] lg:py-12">
      <div className="surface-panel min-w-0 p-5 sm:p-8">
        <Link
          to="/editorials"
          onClick={(event) => {
            if (!editDirty) return
            event.preventDefault()
            void confirm({
              title: '离开编辑页？',
              description: '尚未保存的题解修改会丢失。',
              confirmLabel: '放弃并离开',
              destructive: true,
            }).then((accepted) => accepted && navigate('/editorials'))
          }}
          className="mb-8 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> 返回题解
        </Link>
        <article>
          <header className="border-b border-border pb-6">
            <Link
              to={`/problems/${editorial.problemPublicId || editorial.problemId}`}
              className="text-xs text-primary hover:underline"
            >
              {editorial.problemTitle || '查看原题'}
            </Link>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight sm:text-3xl">
              {editorial.title}
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              {editorial.authorName || '匿名作者'} · {formatDateTime(editorial.createdAt)}
            </p>
          </header>

          {editing ? (
            <form
              className="my-8 flex flex-col gap-5"
              onSubmit={(event) => {
                event.preventDefault()
                void handleSave()
              }}
            >
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="editorial-title">标题</Label>
                <Input
                  id="editorial-title"
                  value={draftTitle}
                  onChange={(event) => setDraftTitle(event.target.value)}
                  maxLength={200}
                  required
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="editorial-content">正文（Markdown 与 LaTeX）</Label>
                <Textarea
                  id="editorial-content"
                  value={draftContent}
                  onChange={(event) => setDraftContent(event.target.value)}
                  rows={20}
                  required
                  className="font-mono text-sm leading-6"
                />
              </div>
              <div className="flex flex-wrap items-end gap-3 border-y border-border py-4">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="editorial-visibility">可见性</Label>
                  <Select
                    value={draftVisibility}
                    onValueChange={(value) => setDraftVisibility(value as 'public' | 'private')}
                  >
                    <SelectTrigger id="editorial-visibility" className="w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="public">公开</SelectItem>
                      <SelectItem value="private">私有</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="editorial-status">状态</Label>
                  <Select
                    value={draftStatus}
                    onValueChange={(value) => setDraftStatus(value as 'draft' | 'published')}
                  >
                    <SelectTrigger id="editorial-status" className="w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="published">已发布</SelectItem>
                      <SelectItem value="draft">草稿</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <label className="flex min-h-9 items-center gap-2 text-sm text-muted-foreground">
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked={draftSolvedOnly}
                    onChange={(event) => setDraftSolvedOnly(event.target.checked)}
                  />
                  通过原题后才能阅读
                </label>
                <div className="ml-auto flex gap-2">
                  <Button type="button" variant="ghost" onClick={() => void cancelEditing()}>
                    取消
                  </Button>
                  <Button type="submit" loading={saving}>
                    <Save /> 保存
                  </Button>
                </div>
              </div>
            </form>
          ) : editorial.locked ? (
            <div className="my-10 border-l-2 border-primary py-2 pl-5">
              <h2 className="flex items-center gap-2 text-base font-semibold">
                <Lock className="size-4" /> 通过本题后解锁正文与讨论
              </h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                作者希望你先独立思考。提交一次 Accepted 后，这篇题解会自动解锁。
              </p>
            </div>
          ) : (
            <MdRenderer content={editorial.contentMd} className="mx-auto mt-8 max-w-3xl" />
          )}

          <div className="mt-8 flex items-center gap-2 border-y border-border py-4">
            <Button
              variant={editorial.voted ? 'secondary' : 'outline'}
              loading={voting}
              disabled={!editorial.permissions.vote}
              onClick={handleVote}
              aria-pressed={editorial.voted}
            >
              <ThumbsUp /> 赞同 {editorial.voteCount}
            </Button>
            {editorial.permissions.edit || editorial.permissions.delete ? (
              <div className="ml-auto flex items-center gap-1">
                {editorial.permissions.edit && (
                  <Button variant="ghost" onClick={beginEditing}>
                    <Pencil /> 编辑
                  </Button>
                )}
                {editorial.permissions.delete && (
                  <Button variant="ghost" className="text-destructive" onClick={handleDelete}>
                    <Trash2 /> 删除
                  </Button>
                )}
              </div>
            ) : null}
          </div>
        </article>

        {editorial.locked ? null : (
          <section className="mt-10" aria-labelledby="editorial-discussion-title">
            <h2
              id="editorial-discussion-title"
              className="mb-5 flex items-center gap-2 text-lg font-semibold"
            >
              <MessageSquare className="size-4" /> 讨论
            </h2>
            <DiscussionSection
              reloadKey={editorial.id}
              fetchPosts={() => listDiscussions(editorial.id)}
              createPost={async (content, parentId) => {
                await createDiscussion(editorial.id, { contentMd: content, parentId })
              }}
              onUpdate={async (postId, content) => {
                await updateDiscussion(postId, { contentMd: content })
              }}
              onDelete={async (postId) => {
                await deleteDiscussion(postId)
              }}
            />
          </section>
        )}
      </div>

      <aside className="hidden border-l border-border pl-6 text-sm lg:block">
        <div className="sticky top-24">
          <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            关于这篇题解
          </h2>
          <dl className="space-y-3">
            <div>
              <dt className="text-xs text-muted-foreground">赞同</dt>
              <dd className="mt-0.5 font-mono">{editorial.voteCount}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">可见性</dt>
              <dd className="mt-0.5">{editorial.solvedOnly ? '通过后可见' : '公开'}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">最后更新</dt>
              <dd className="mt-0.5">{formatDateTime(editorial.updatedAt)}</dd>
            </div>
          </dl>
        </div>
      </aside>
    </div>
  )
}
