import { useEffect, useState } from 'react'
import { Alert, Card, Table, Button, Tag, Typography, Space, message, Tabs, Skeleton, Empty, Input, Modal } from 'antd'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  getApiContestsId as getContest,
  getApiContestsIdRankboard as getContestRankboard,
  getApiContestsIdRegistration as getContestRegistration,
  postApiContestsIdRegister as registerContest,
} from '../generated/api/vertex'
import type {
  DtoContestProblemResponse as ContestProblem,
  DtoContestResponse as Contest,
  DtoRankboardResponse as Rankboard,
  DtoRankboardRowResponse as RankRow,
} from '../generated/api/model'
import { useAuth } from '../auth/AuthContext'

// ContestDetailPage:比赛详情(信息 + 题目 + 实时榜单 + 注册)。
export default function ContestDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { user } = useAuth()
  const [contest, setContest] = useState<Contest | null>(null)
  const [problems, setProblems] = useState<ContestProblem[]>([])
  const [board, setBoard] = useState<Rankboard | null>(null)
  const [loading, setLoading] = useState(true)
  const [registered, setRegistered] = useState(false)
  const [registering, setRegistering] = useState(false)
  const [passwordOpen, setPasswordOpen] = useState(false)
  const [contestPassword, setContestPassword] = useState('')

  async function loadBoard(frozen?: boolean) {
    if (!id) return
    try {
      setBoard(await getContestRankboard(id, frozen === undefined ? {} : { frozen }))
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
      if (user) {
        const registration = await getContestRegistration(id)
        setRegistered(registration.registered)
      } else {
        setRegistered(false)
      }
      if (res.contest.rankboardVisible) await loadBoard()
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '比赛加载失败')
      setContest(null)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [id, user?.id])

  // 比赛进行中轮询榜单(实时更新)
  useEffect(() => {
    const timer = setInterval(() => {
      if (contest?.rankboardVisible && new Date() < new Date(contest.endAt)) {
        loadBoard()
      }
    }, 5000)
    return () => clearInterval(timer)
  }, [contest])

  async function handleRegister(password?: string) {
    if (!id) return
    if (!user) {
      message.warning('请先登录')
      navigate('/login', { state: { from: `/contests/${id}` } })
      return
    }
    if (contest?.visibility === 'password' && password === undefined) {
      setPasswordOpen(true)
      return
    }
    setRegistering(true)
    try {
      await registerContest(id, password ? { password } : {})
      message.success('注册成功')
      setRegistered(true)
      setPasswordOpen(false)
      setContestPassword('')
      await load()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '注册失败')
    } finally {
      setRegistering(false)
    }
  }

  if (loading) return <Skeleton active />
  if (!contest) return <Card>比赛不存在</Card>

  const now = Date.now()
  const hasStarted = now >= new Date(contest.beginAt).getTime()
  const hasEnded = now > new Date(contest.endAt).getTime()
  const canRegister = !hasStarted && !registered

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
            <Button type="primary" loading={registering} onClick={() => handleRegister()} disabled={!canRegister}>
              {registered ? '已注册' : hasEnded ? '比赛已结束' : hasStarted ? '比赛已开始' : '注册比赛'}
            </Button>
            <Link to={`/submissions?contest=${contest.id}`}>查看比赛提交记录</Link>
          </Space>
        </Card>
      ),
    },
    {
      key: 'problems',
      label: '题目',
      children: (
        <Table<ContestProblem>
          rowKey="problemId"
          dataSource={problems}
          pagination={false}
          locale={{
            emptyText: user?.role === 'admin'
              ? '尚未组题，请到比赛管理中添加题目'
              : contest.visibility === 'password' && !registered
                ? '报名后可查看比赛题目'
                : '暂未公布题目',
          }}
          columns={[
            { title: '#', dataIndex: 'sortOrder', width: 60, render: (i: number) => <Tag>{String.fromCharCode(65 + i)}</Tag> },
            {
              title: '题目',
              dataIndex: 'title',
              render: (title: string, problem) => <Link to={`/problems/${problem.problemId}?contest=${id}`}>{title}</Link>,
            },
            { title: '难度', dataIndex: 'difficulty', width: 90, align: 'center' },
            {
              title: '标签',
              dataIndex: 'tags',
              render: (tags: string[]) => tags?.map((tag) => <Tag key={tag}>{tag}</Tag>),
            },
          ]}
        />
      ),
    },
    {
      key: 'rankboard',
      label: board?.frozen ? '榜单(已封榜)' : '实时榜单',
      children: !contest.rankboardVisible ? (
        <Alert type="info" showIcon message="该比赛未公开榜单" />
      ) : board ? (
        <RankboardTable board={board} />
      ) : (
        <Empty description="暂无榜单" />
      ),
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
      <Modal
        title="输入比赛密码"
        open={passwordOpen}
        okText="报名"
        confirmLoading={registering}
        onOk={() => handleRegister(contestPassword)}
        onCancel={() => {
          setPasswordOpen(false)
          setContestPassword('')
        }}
      >
        <Input.Password
          autoFocus
          value={contestPassword}
          onChange={(event) => setContestPassword(event.target.value)}
          onPressEnter={() => contestPassword && handleRegister(contestPassword)}
          placeholder="比赛密码"
        />
      </Modal>
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
            if (cell.pendingCount > 0) return <Tag color="gold">?</Tag>
            if (cell.attempts > 0) return <Typography.Text type="danger">{`-${cell.attempts}`}</Typography.Text>
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
