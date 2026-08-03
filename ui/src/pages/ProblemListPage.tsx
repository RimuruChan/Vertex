import { useEffect, useState } from 'react'
import { Table, Card, Input, message, Select, Tag, Space, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getApiProblems as listProblems } from '../generated/api/vertex'
import type { DtoProblemResponse as Problem } from '../generated/api/model'

// ProblemListPage:题目列表(难度/标签/关键字筛选 + 分页)。
export default function ProblemListPage() {
  const navigate = useNavigate()
  const [data, setData] = useState<Problem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [size] = useState(20)
  const [keyword, setKeyword] = useState('')
  const [difficulty, setDifficulty] = useState<number | undefined>()
  const [tag, setTag] = useState<string>()
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const res = await listProblems({ page, size, keyword: keyword || undefined, difficulty, tag })
      setData(res.items)
      setTotal(res.total)
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '题库加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page, difficulty, keyword, tag])

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
              setKeyword(v.trim())
            }}
            style={{ width: 220 }}
          />
          {tag && <Tag closable onClose={() => { setPage(1); setTag(undefined) }}>标签：{tag}</Tag>}
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
          { title: '标题', dataIndex: 'title', render: (title: string) => <Typography.Text strong>{title}</Typography.Text> },
          { title: '难度', dataIndex: 'difficulty', width: 100, render: (value: number) => <Tag color={value <= 3 ? 'green' : value <= 6 ? 'gold' : 'red'}>难度 {value}</Tag> },
          {
            title: '标签',
            dataIndex: 'tags',
            width: 220,
            render: (tags: string[]) =>
              tags?.map((t) => (
                <Tag
                  key={t}
                  color="blue"
                  style={{ cursor: 'pointer' }}
                  onClick={(event) => {
                    event.stopPropagation()
                    setPage(1)
                    setTag(t)
                  }}
                >
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
