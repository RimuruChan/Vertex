import { useEffect, useState } from 'react'
import { Card, Table, Typography, Tag, Select } from 'antd'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { listSubmissions } from '../api/client'
import type { Submission } from '../api/types'
import VerdictTag from '../components/VerdictTag'

const statusOptions = [
  { value: 'Accepted', label: 'AC' },
  { value: 'Wrong Answer', label: 'WA' },
  { value: 'Time Limit Exceeded', label: 'TLE' },
  { value: 'Memory Limit Exceeded', label: 'MLE' },
  { value: 'Runtime Error', label: 'RE' },
  { value: 'Compile Error', label: 'CE' },
]

// SubmissionListPage:提交记录列表。
export default function SubmissionListPage() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [data, setData] = useState<Submission[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string | undefined>(params.get('status') ?? undefined)
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const problem = params.get('problem') ?? undefined
      const user = params.get('user') ?? undefined
      const res = await listSubmissions({ page, size: 20, status, problem, user })
      setData(res.items)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page, status, params])

  return (
    <Card
      title="提交记录"
      extra={
        <Select
          placeholder="按判定筛选"
          allowClear
          style={{ width: 120 }}
          options={statusOptions}
          onChange={(v) => {
            setPage(1)
            setStatus(v)
          }}
        />
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
          { title: '用户', dataIndex: 'username', width: 120, render: (u: string) => <Link to={`/submissions?user=${u}`}>{u}</Link> },
          {
            title: '题目',
            dataIndex: 'problemTitle',
            ellipsis: true,
            render: (t: string, r) => <Link to={`/problems/${r.problemId}`}>{t}</Link>,
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
