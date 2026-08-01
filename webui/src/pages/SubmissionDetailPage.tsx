import { useEffect, useState } from 'react'
import { Card, Descriptions, Table, Tag, Space } from 'antd'
import { useParams, Link } from 'react-router-dom'
import { getSubmission } from '../api/client'
import type { Submission, CaseResult } from '../api/types'
import VerdictTag from '../components/VerdictTag'

// SubmissionDetailPage:提交详情(含逐测试点结果)。
export default function SubmissionDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [sub, setSub] = useState<Submission | null>(null)
  const [loading, setLoading] = useState(true)

  async function load() {
    if (!id) return
    setLoading(true)
    try {
      setSub(await getSubmission(id))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [id])

  // 评测中轮询(后端 WS 就绪前,轮询兜底)
  useEffect(() => {
    const timer = setInterval(() => {
      if (sub && (sub.status === 'Pending' || sub.status === 'Judging')) {
        load()
      }
    }, 2000)
    return () => clearInterval(timer)
  }, [sub])

  if (!sub) return <Card loading={loading}>加载中...</Card>

  const caseTableData: CaseResult[] = sub.caseResults ?? []

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card title={`提交 #${sub.id.slice(0, 8)}`}>
        <Descriptions size="small" column={4}>
          <Descriptions.Item label="题目">
            <Link to={`/problems/${sub.problemId}`}>{sub.problemTitle}</Link>
          </Descriptions.Item>
          <Descriptions.Item label="用户">{sub.username}</Descriptions.Item>
          <Descriptions.Item label="语言">
            <Tag>{sub.language}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="判定">
            <VerdictTag status={sub.status} />
          </Descriptions.Item>
          <Descriptions.Item label="总时间">{sub.totalTimeMs} ms</Descriptions.Item>
          <Descriptions.Item label="峰值内存">
            {(sub.peakMemoryKb / 1024).toFixed(1)} MB
          </Descriptions.Item>
          <Descriptions.Item label="提交时间" span={2}>
            {new Date(sub.submittedAt).toLocaleString()}
          </Descriptions.Item>
        </Descriptions>
      </Card>

      {sub.status === 'Compile Error' && (
        <Card title="编译信息">
          <pre style={{ whiteSpace: 'pre-wrap', background: '#fafafa', padding: 12, borderRadius: 6 }}>
            {sub.compileResult}
          </pre>
        </Card>
      )}

      {caseTableData.length > 0 && (
        <Card title="测试点结果">
          <Table<CaseResult>
            rowKey="caseIndex"
            size="small"
            dataSource={caseTableData}
            pagination={false}
            columns={[
              { title: '测试点', dataIndex: 'caseIndex', width: 80, render: (i: number) => `#${i}` },
              { title: '判定', dataIndex: 'verdict', width: 120, render: (v: string) => <VerdictTag status={v} /> },
              { title: '时间', dataIndex: 'timeMs', width: 100, align: 'right', render: (t: number) => `${t} ms` },
              { title: '内存', dataIndex: 'memoryKb', width: 110, align: 'right', render: (m: number) => `${(m / 1024).toFixed(1)} MB` },
              {
                title: '详情',
                dataIndex: 'checkerOutput',
                ellipsis: true,
                render: (o: string, r) => o || r.exitStatus || '—',
              },
            ]}
          />
        </Card>
      )}

      {sub.sourceCode && (
        <Card title="源代码">
          <pre
            style={{
              background: '#282c34',
              color: '#e0e0e0',
              padding: 16,
              borderRadius: 6,
              overflow: 'auto',
              fontSize: 13,
            }}
          >
            {sub.sourceCode}
          </pre>
        </Card>
      )}
    </Space>
  )
}
