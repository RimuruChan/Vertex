import { useEffect, useState } from 'react'
import { Card, Descriptions, Select, Button, Space, message, Skeleton } from 'antd'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { getProblem, submit, currentUser } from '../api/client'
import type { Problem, Submission } from '../api/types'
import MdRenderer from '../components/MdRenderer'
import CodeEditor, { languageTemplates } from '../components/CodeEditor'
import VerdictTag from '../components/VerdictTag'

// ProblemDetailPage:题目详情 + 提交面板。
export default function ProblemDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
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
    if (!currentUser()) {
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
      const sub = await submit({ problemId: id, language, sourceCode: code })
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

  return (
    <div>
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

      <Card title="提交代码" style={{ marginTop: 16 }}>
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
    </div>
  )
}
