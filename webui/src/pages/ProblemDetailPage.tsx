import { useEffect, useState } from 'react'
import { Card, Descriptions, Select, Button, Space, message, Skeleton, Tabs, Typography, Input } from 'antd'
import { useParams, useNavigate, Link, useSearchParams } from 'react-router-dom'
import {
  deleteApiDiscussionsId as deleteDiscussion,
  getApiEditorials as listEditorials,
  getApiProblemsId as getProblem,
  getApiProblemsIdDiscussions as listProblemDiscussions,
  postApiEditorials as createEditorial,
  postApiProblemsIdDiscussions as createProblemDiscussion,
  postApiSubmissions as submit,
} from '../generated/api/vertex'
import type {
  DtoEditorialResponse as Editorial,
  DtoProblemResponse as Problem,
  DtoSubmissionResponse as Submission,
} from '../generated/api/model'
import { useAuth } from '../auth/AuthContext'
import MdRenderer from '../components/MdRenderer'
import CodeEditor, { languageTemplates } from '../components/CodeEditor'
import VerdictTag from '../components/VerdictTag'
import DiscussionSection from '../components/DiscussionSection'

// ProblemDetailPage:题目详情 + 提交面板。
export default function ProblemDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user } = useAuth()
  const [searchParams] = useSearchParams()
  const contestId = searchParams.get('contest') ?? undefined
  const [problem, setProblem] = useState<Problem | null>(null)
  const [loading, setLoading] = useState(true)
  const [language, setLanguage] = useState('cpp')
  const [code, setCode] = useState(languageTemplates.cpp)
  const [submitting, setSubmitting] = useState(false)
  const [lastSubmission, setLastSubmission] = useState<Submission | null>(null)

  async function load() {
    if (!id) return
    setLoading(true)
    try {
      const p = await getProblem(id)
      setProblem(p)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '题目不存在')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [id])

  async function handleSubmit() {
    if (!id) return
    if (!user) {
      message.warning('请先登录')
      navigate('/login')
      return
    }
    if (!code.trim()) {
      message.warning('请输入代码')
      return
    }
    setSubmitting(true)
    try {
      const sub = await submit({ problemId: id, language, sourceCode: code, contestId })
      setLastSubmission(sub)
      message.success('提交成功,等待评测')
      navigate(`/submissions/${sub.id}`)
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '提交失败')
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) return <Skeleton active />
  if (!problem) return <Card>题目不存在</Card>

  const tabs = [
    {
      key: 'statement',
      label: '题面',
      children: (
        <Card
          title={
            <Space>
              <span>{problem.title}</span>
              {lastSubmission && <VerdictTag status={lastSubmission.status} />}
            </Space>
          }
          extra={<Link to={`/submissions?problem=${problem.id}`}>查看该题提交记录</Link>}
        >
          <Descriptions size="small" column={4} style={{ marginBottom: 16 }}>
            <Descriptions.Item label="时间限制">{problem.timeLimitMs} ms</Descriptions.Item>
            <Descriptions.Item label="内存限制">
              {(problem.memoryLimitKb / 1024).toFixed(0)} MB
            </Descriptions.Item>
            <Descriptions.Item label="提交">{problem.submissionCount}</Descriptions.Item>
            <Descriptions.Item label="通过率">
              {problem.submissionCount > 0
                ? `${((problem.acceptedCount / problem.submissionCount) * 100).toFixed(1)}%`
                : '—'}
            </Descriptions.Item>
          </Descriptions>

          <div className="markdown-body">
            <MdRenderer content={problem.statementMd} />
          </div>
        </Card>
      ),
    },
    {
      key: 'submit',
      label: '提交',
      children: (
        <Card title="提交代码">
          <Space style={{ marginBottom: 12 }}>
            <Select
              value={language}
              style={{ width: 140 }}
              onChange={(v: string) => {
                setLanguage(v)
                setCode(languageTemplates[v] ?? '')
              }}
              options={[
                { value: 'cpp', label: 'C++17' },
                { value: 'c', label: 'C11' },
                { value: 'python', label: 'Python 3' },
              ]}
            />
            <Button onClick={() => setCode(languageTemplates[language] ?? '')}>重置模板</Button>
          </Space>
          <CodeEditor value={code} onChange={setCode} language={language} height={360} />
          <Space style={{ marginTop: 12 }}>
            <Button type="primary" loading={submitting} onClick={handleSubmit}>
              提交
            </Button>
            <Button onClick={() => setCode('')}>清空</Button>
          </Space>
        </Card>
      ),
    },
    {
      key: 'editorials',
      label: '题解',
      children: <EditorialSection problemId={problem.id} />,
    },
    {
      key: 'discussions',
      label: '讨论',
      children: (
        <DiscussionSection
          fetchPosts={async () => (await listProblemDiscussions(problem.id)).items}
          createPost={async (content, parentId) => {
            await createProblemDiscussion(problem.id, { contentMd: content, parentId })
          }}
          onDelete={async (postID) => {
            await deleteDiscussion(postID)
          }}
        />
      ),
    },
  ]

  return <Tabs items={tabs} />
}

// EditorialSection:某题的题解列表 + 发布。
function EditorialSection({ problemId }: { problemId: string }) {
  const { user } = useAuth()
  const [editorials, setEditorials] = useState<Editorial[]>([])
  const [showForm, setShowForm] = useState(false)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function load() {
    const res = await listEditorials({ problem: problemId })
    setEditorials(res.items)
  }

  useEffect(() => {
    load()
  }, [problemId])

  async function handlePublish() {
    if (!title.trim() || !content.trim()) {
      message.warning('请填写标题与内容')
      return
    }
    setSubmitting(true)
    try {
      await createEditorial({ problemId, title: title.trim(), contentMd: content.trim() })
      message.success('题解已发布')
      setShowForm(false)
      setTitle('')
      setContent('')
      await load()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '发布失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card
      title={`题解 (${editorials.length})`}
      extra={
        user && (
          <Button size="small" onClick={() => setShowForm(!showForm)}>
            {showForm ? '取消' : '发布题解'}
          </Button>
        )
      }
    >
      {showForm && (
        <div style={{ marginBottom: 24 }}>
          <Input
            placeholder="题解标题"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            style={{ marginBottom: 8 }}
          />
          <Input.TextArea
            rows={6}
            placeholder="题解内容,支持 Markdown 与 LaTeX"
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
          <Button type="primary" loading={submitting} onClick={handlePublish} style={{ marginTop: 8 }}>
            发布
          </Button>
        </div>
      )}
      {editorials.length === 0 ? (
        <Typography.Text type="secondary">还没有题解,来发布第一篇吧</Typography.Text>
      ) : (
        <Space direction="vertical" style={{ width: '100%' }}>
          {editorials.map((e) => (
            <Card key={e.id} size="small" type="inner">
              <Typography.Title level={5}>{e.title}</Typography.Title>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {e.authorName} · {new Date(e.createdAt).toLocaleDateString()}
              </Typography.Text>
              <div className="markdown-body" style={{ marginTop: 12 }}>
                <MdRenderer content={e.contentMd} />
              </div>
            </Card>
          ))}
        </Space>
      )}
    </Card>
  )
}
