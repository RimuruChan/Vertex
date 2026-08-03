import { useEffect, useState } from 'react'
import { Card, message, Table, Tag, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getApiContests as listContests } from '../generated/api/vertex'
import type { DtoContestResponse as Contest } from '../generated/api/model'

// ContestListPage:比赛列表(M4 完善)。
export default function ContestListPage() {
  const navigate = useNavigate()
  const [data, setData] = useState<Contest[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const res = await listContests({ page, size: 20 })
      setData(res.items)
      setTotal(res.total)
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '比赛列表加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page])

  function statusTag(c: Contest) {
    const now = Date.now()
    if (now < new Date(c.beginAt).getTime()) return <Tag color="blue">未开始</Tag>
    if (now > new Date(c.endAt).getTime()) return <Tag>已结束</Tag>
    return <Tag color="green">进行中</Tag>
  }

  return (
    <Card title="比赛">
      <Table<Contest>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={{ current: page, pageSize: 20, total, onChange: setPage, showSizeChanger: false }}
        onRow={(r) => ({ onClick: () => navigate(`/contests/${r.id}`), style: { cursor: 'pointer' } })}
        columns={[
          {
            title: '标题',
            dataIndex: 'title',
            ellipsis: true,
            render: (title: string, contest) => (
              <span>{title} {contest.visibility === 'password' && <Tag color="gold">密码赛</Tag>}</span>
            ),
          },
          { title: '赛制', dataIndex: 'rule', width: 100, render: (r: string) => <Tag>{r === 'acm' ? 'ACM' : 'IOI'}</Tag> },
          { title: '状态', key: 'status', width: 100, render: (_, c) => statusTag(c) },
          {
            title: '开始时间',
            dataIndex: 'beginAt',
            width: 180,
            render: (t: string) => <Typography.Text type="secondary">{new Date(t).toLocaleString()}</Typography.Text>,
          },
          {
            title: '结束时间',
            dataIndex: 'endAt',
            width: 180,
            render: (t: string) => <Typography.Text type="secondary">{new Date(t).toLocaleString()}</Typography.Text>,
          },
        ]}
      />
    </Card>
  )
}
