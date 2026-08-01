import { useEffect, useState } from 'react'
import { Table, Card, Input, Select, Tag, Space, Typography, Rate } from 'antd'
import { Link, useNavigate } from 'react-router-dom'
import { listProblems } from '../api/client'
import type { Problem } from '../api/types'

// ProblemListPage:题目列表(难度/标签/关键字筛选 + 分页)。
export default function ProblemListPage() {
  const navigate = useNavigate()
  const [data, setData] = useState<Problem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [size] = useState(20)
  const [keyword, setKeyword] = useState('')
  const [difficulty, setDifficulty] = useState<number | undefined>()
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const res = await listProblems({ page, size, keyword: keyword || undefined, difficulty })
      setData(res.items)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page, difficulty])

  return (
    <Card
      title="题库"
      extra={
        <Space>
          <Input.Search
            placeholder="搜索题目"
            allowClear
            onSearch={(v) => {
              setPage(1)
              setKeyword(v)
              load()
            }}
            style={{ width: 220 }}
          />
          <Select
            placeholder="难度"
            allowClear
            style={{ width: 100 }}
            options={[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((d) => ({ value: d, label: `难度${d}` }))}
            onChange={(v) => {
              setPage(1)
              setDifficulty(v)
            }}
          />
        </Space>
      }
    >
      <Table<Problem>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={{
          current: page,
          pageSize: size,
          total,
          onChange: setPage,
          showSizeChanger: false,
        }}
        onRow={(record) => ({
          onClick: () => navigate(`/problems/${record.id}`),
          style: { cursor: 'pointer' },
        })}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 80, render: (id: string) => <Typography.Text type="secondary">#{id.slice(0, 8)}</Typography.Text> },
          { title: '标题', dataIndex: 'title', render: (t: string) => <Link to="#">{t}</Link> },
          { title: '难度', dataIndex: 'difficulty', width: 120, render: (d: number) => <Rate disabled value={d} count={d} /> },
          {
            title: '标签',
            dataIndex: 'tags',
            width: 220,
            render: (tags: string[]) =>
              tags?.map((t) => (
                <Tag key={t} color="blue">
                  {t}
                </Tag>
              )),
          },
          { title: '提交', dataIndex: 'submissionCount', width: 80, align: 'center' },
          {
            title: '通过率',
            key: 'ratio',
            width: 100,
            align: 'center',
            render: (_, p) =>
              p.submissionCount > 0 ? `${((p.acceptedCount / p.submissionCount) * 100).toFixed(1)}%` : '—',
          },
        ]}
      />
    </Card>
  )
}
