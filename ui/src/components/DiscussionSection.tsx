import { useEffect, useState } from 'react'
import { Card, Input, Button, Space, List, message, Popconfirm, Typography } from 'antd'
import MdRenderer from './MdRenderer'
import type { DtoDiscussionResponse as DiscussionPost } from '../generated/api/model'
import { useAuth } from '../auth/AuthContext'

// DiscussionSection:通用评论区(题目/题解复用)。
// fetchPosts 加载评论;createPost 发表评论(可选 parentId 支持回复)。
export default function DiscussionSection({
  fetchPosts,
  createPost,
  onDelete,
}: {
  fetchPosts: () => Promise<DiscussionPost[]>
  createPost: (content: string, parentId?: number) => Promise<void>
  onDelete?: (id: number) => Promise<void>
}) {
  const [posts, setPosts] = useState<DiscussionPost[]>([])
  const [content, setContent] = useState('')
  const [replyingTo, setReplyingTo] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const { user } = useAuth()

  async function load() {
    setLoading(true)
    try {
      setPosts(await fetchPosts())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const canPost = !!user

  async function handleSubmit() {
    if (!content.trim()) {
      message.warning('请输入内容')
      return
    }
    setSubmitting(true)
    try {
      await createPost(content.trim(), replyingTo ?? undefined)
      setContent('')
      setReplyingTo(null)
      message.success('已发表')
      await load()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '发表失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card
      title={`讨论区 (${posts.length})`}
      extra={
        canPost && (
          <Button size="small" onClick={() => setReplyingTo(null)}>
            新评论
          </Button>
        )
      }
    >
      {canPost && (
        <div style={{ marginBottom: 16 }}>
          <Input.TextArea
            rows={3}
            placeholder="写下你的看法,支持 Markdown 与 LaTeX($x^2$)"
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
          <Space style={{ marginTop: 8 }}>
            {replyingTo !== null && (
              <Typography.Text type="secondary">回复评论 #{replyingTo}</Typography.Text>
            )}
            <Button type="primary" loading={submitting} onClick={handleSubmit}>
              发表
            </Button>
          </Space>
        </div>
      )}

      <List<DiscussionPost>
        loading={loading}
        dataSource={posts}
        locale={{ emptyText: '暂无评论' }}
        renderItem={(post) => (
          <List.Item
            actions={[
              canPost && (
                <Button key="reply" size="small" type="link" onClick={() => setReplyingTo(post.id)}>
                  回复
                </Button>
              ),
              (onDelete && (post.authorId === user?.id || user?.role === 'admin')) && (
                <Popconfirm
                  key="del"
                  title="确认删除这条评论?"
                  onConfirm={async () => {
                    await onDelete(post.id)
                    message.success('已删除')
                    await load()
                  }}
                >
                  <Button size="small" type="link" danger>
                    删除
                  </Button>
                </Popconfirm>
              ),
            ].filter(Boolean)}
          >
            <List.Item.Meta
              title={
                <Space>
                  <Typography.Text strong>{post.authorName}</Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {new Date(post.createdAt).toLocaleString()}
                  </Typography.Text>
                </Space>
              }
              description={
                <div className="markdown-body" style={{ marginTop: 8 }}>
                  <MdRenderer content={post.contentMd} />
                </div>
              }
            />
          </List.Item>
        )}
      />
    </Card>
  )
}
