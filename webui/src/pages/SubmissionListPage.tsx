import { useEffect, useState } from 'react'
import { Button, Card, message, Select, Space, Table, Tag, Typography } from 'antd'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { getApiSubmissions as listSubmissions } from '../generated/api/vertex'
import type { DtoSubmissionResponse as Submission } from '../generated/api/model'
import VerdictTag from '../components/VerdictTag'
import { useAuth } from '../auth/AuthContext'

const statusOptions = [
  { value: 'Pending', label: '等待评测' },
  { value: 'Judging', label: '评测中' },
  { value: 'Accepted', label: 'AC' },
  { value: 'Wrong Answer', label: 'WA' },
  { value: 'Time Limit Exceeded', label: 'TLE' },
  { value: 'Memory Limit Exceeded', label: 'MLE' },
  { value: 'Runtime Error', label: 'RE' },
  { value: 'Compile Error', label: 'CE' },
  { value: 'Output Limit Exceeded', label: 'OLE' },
  { value: 'System Error', label: 'SE' },
]

// SubmissionListPage:提交记录列表。
export default function SubmissionListPage() {
  const navigate = useNavigate()
  const { user } = useAuth()
  const [params] = useSearchParams()
  const [data, setData] = useState<Submission[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string | undefined>(params.get('status') ?? undefined)
  const [language, setLanguage] = useState<string | undefined>(params.get('language') ?? undefined)
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const problem = params.get('problem') ?? undefined
      const user = params.get('user') ?? undefined
      const contest = params.get('contest') ?? undefined
      const res = await listSubmissions({ page, size: 20, status, language, problem, user, contest })
      setData(res.items)
      setTotal(res.total)
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '提交记录加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page, status, language, params])

  return (
    <Card
      title="提交记录"
      extra={
        <Space wrap>
          <Button onClick={() => navigate(`/submissions?user=${user?.username}`)}>我的提交</Button>
          <Select
            value={language}
            placeholder="按语言筛选"
            allowClear
            style={{ width: 120 }}
            options={[
              { value: 'cpp', label: 'C++17' },
              { value: 'c', label: 'C11' },
              { value: 'python', label: 'Python 3' },
            ]}
            onChange={(value) => {
              setPage(1)
              setLanguage(value)
            }}
          />
          <Select
            value={status}
            placeholder="按判定筛选"
            allowClear
            style={{ width: 150 }}
            options={statusOptions}
            onChange={(value) => {
              setPage(1)
              setStatus(value)
            }}
          />
        </Space>
      }
    >
      <Table<Submission>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={{ current: page, pageSize: 20, total, onChange: setPage, showSizeChanger: false }}
        onRow={(r) => ({ onClick: () => navigate(`/submissions/${r.id}`), style: { cursor: 'pointer' } })}
        columns={[
          {
            title: 'ID',
            dataIndex: 'id',
            width: 90,
            render: (id: string) => <Typography.Text type="secondary">#{id.slice(0, 8)}</Typography.Text>,
          },
          {
            title: '用户',
            dataIndex: 'username',
            width: 120,
            render: (username: string) => <Link to={`/submissions?user=${username}`} onClick={(event) => event.stopPropagation()}>{username}</Link>,
          },
          {
            title: '题目',
            dataIndex: 'problemTitle',
            ellipsis: true,
            render: (title: string, record) => <Link to={`/problems/${record.problemId}`} onClick={(event) => event.stopPropagation()}>{title}</Link>,
          },
          { title: '语言', dataIndex: 'language', width: 90, render: (l: string) => <Tag>{l}</Tag> },
          { title: '判定', dataIndex: 'status', width: 110, render: (s: string) => <VerdictTag status={s} /> },
          { title: '时间', dataIndex: 'totalTimeMs', width: 90, align: 'right', render: (t: number) => `${t} ms` },
          { title: '内存', dataIndex: 'peakMemoryKb', width: 100, align: 'right', render: (m: number) => `${(m / 1024).toFixed(0)} MB` },
          { title: '提交时间', dataIndex: 'submittedAt', width: 160, render: (t: string) => new Date(t).toLocaleString() },
        ]}
      />
    </Card>
  )
}
