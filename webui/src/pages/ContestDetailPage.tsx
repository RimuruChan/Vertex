import { useEffect, useState } from 'react'
import { Card, Table, Button, Tag, Typography, Space, message, Tabs, Skeleton, Empty } from 'antd'
import { useParams, Link } from 'react-router-dom'
import { getContest, getContestRankboard, registerContest, currentUser } from '../api/client'
import type { Contest, Rankboard, RankRow } from '../api/types'

// 后端返回的比赛题目关联(contestId 非必须)
interface ContestProblemItem {
  problemId: string
  sortOrder: number
}

// ContestDetailPage:比赛详情(信息 + 题目 + 实时榜单 + 注册)。
export default function ContestDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [contest, setContest] = useState<Contest | null>(null)
  const [problems, setProblems] = useState<ContestProblemItem[]>([])
  const [board, setBoard] = useState<Rankboard | null>(null)
  const [loading, setLoading] = useState(true)

  async function loadBoard(frozen?: boolean) {
    if (!id) return
    try {
      setBoard(await getContestRankboard(id, frozen))
    } catch {
      // 榜单可能尚未生成
    }
  }

  async function load() {
    if (!id) return
    setLoading(true)
    try {
      const res = await getContest(id)
      setContest(res.contest)
      setProblems(res.problems)
      await loadBoard()
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [id])

  // 比赛进行中轮询榜单(实时更新)
  useEffect(() => {
    const timer = setInterval(() => {
      if (contest && new Date() < new Date(contest.endAt)) {
        loadBoard()
      }
    }, 5000)
    return () => clearInterval(timer)
  }, [contest])

  async function handleRegister() {
    if (!id) return
    if (!currentUser()) {
      message.warning('请先登录')
      return
    }
    try {
      await registerContest(id)
      message.success('注册成功')
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '注册失败')
    }
  }

  if (loading) return <Skeleton active />
  if (!contest) return <Card>比赛不存在</Card>

  const tabs = [
    {
      key: 'overview',
      label: '比赛信息',
      children: (
        <Card>
          <Typography.Paragraph>{contest.description || '暂无描述'}</Typography.Paragraph>
          <Space direction="vertical">
            <Typography.Text>开始: {new Date(contest.beginAt).toLocaleString()}</Typography.Text>
            <Typography.Text>结束: {new Date(contest.endAt).toLocaleString()}</Typography.Text>
            {contest.freezeAt && <Typography.Text>封榜: {new Date(contest.freezeAt).toLocaleString()}</Typography.Text>}
            <Button type="primary" onClick={handleRegister}>
              注册比赛
            </Button>
          </Space>
        </Card>
      ),
    },
    {
      key: 'problems',
      label: '题目',
      children: (
        <Table<ContestProblemItem>
          rowKey="problemId"
          dataSource={problems}
          pagination={false}
          columns={[
            { title: '#', dataIndex: 'sortOrder', width: 60, render: (i: number) => <Tag>{String.fromCharCode(65 + i)}</Tag> },
            { title: '题目', dataIndex: 'problemId', render: (pid: string, _: ContestProblemItem) => <Link to={`/problems/${pid}`}>{pid.slice(0, 8)}</Link> },
          ]}
        />
      ),
    },
    {
      key: 'rankboard',
      label: board?.frozen ? '榜单(已封榜)' : '实时榜单',
      children: board ? <RankboardTable board={board} /> : <Empty description="暂无榜单" />,
    },
  ]

  return (
    <div>
      <Card title={contest.title} style={{ marginBottom: 16 }}>
        <Space>
          <Tag color={contest.rule === 'acm' ? 'geekblue' : 'purple'}>{contest.rule === 'acm' ? 'ACM/ICPC' : 'IOI'}</Tag>
          <Tag>{contest.visibility}</Tag>
        </Space>
      </Card>
      <Tabs items={tabs} />
    </div>
  )
}

// RankboardTable:ACM 实时榜单。
export function RankboardTable({ board }: { board: Rankboard }) {
  const problemLetters = board.problemIds.map((_: string, i: number) => String.fromCharCode(65 + i))

  return (
    <Table<RankRow>
      rowKey="userId"
      dataSource={board.rows}
      size="small"
      pagination={false}
      columns={[
        { title: '排名', dataIndex: 'rank', width: 60, align: 'center' },
        { title: '用户', dataIndex: 'username', width: 140 },
        { title: '通过', dataIndex: 'solved', width: 60, align: 'center' },
        { title: '罚时', dataIndex: 'penalty', width: 80, align: 'center', render: (p: number) => formatPenalty(p) },
        ...problemLetters.map((letter: string, i: number) => ({
          title: letter,
          width: 90,
          align: 'center' as const,
          render: (_: unknown, row: RankRow) => {
            const cell = row.cells[i]
            if (!cell) return <Typography.Text type="secondary">·</Typography.Text>
            if (cell.solvedAt) return <Tag color="green">{`${formatMinutes(cell.penaltySec)}`}</Tag>
            if (cell.attempts > 0) return <Typography.Text type="danger">{`-${cell.attempts}`}</Typography.Text>
            if (cell.pendingCount > 0) return <Tag color="gold">?</Tag>
            return <Typography.Text type="secondary">·</Typography.Text>
          },
        })),
      ]}
    />
  )
}

function formatPenalty(sec: number): string {
  return `${Math.floor(sec / 60)} min`
}

function formatMinutes(sec: number): string {
  return `${Math.floor(sec / 60)}`
}
