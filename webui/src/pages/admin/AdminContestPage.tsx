import { useEffect, useMemo, useState } from 'react'
import { Button, Card, DatePicker, Form, Input, List, message, Modal, Select, Space, Switch, Table, Tag, Typography } from 'antd'
import { ArrowDownOutlined, ArrowUpOutlined, DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  getApiAdminContests as adminListContests,
  getApiAdminContestsId as adminGetContest,
  getApiAdminProblems as adminListProblems,
  postApiAdminContests as adminCreateContest,
  putApiAdminContestsId as adminUpdateContest,
  putApiAdminContestsIdProblems as adminSetContestProblems,
} from '../../generated/api/vertex'
import type {
  DtoContestResponse as Contest,
  DtoProblemResponse as Problem,
} from '../../generated/api/model'

const pageSize = 20

export default function AdminContestPage() {
  const [metadataOpen, setMetadataOpen] = useState(false)
  const [metadataContest, setMetadataContest] = useState<Contest | null>(null)
  const [editingContest, setEditingContest] = useState<Contest | null>(null)
  const [data, setData] = useState<Contest[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()
  const formVisibility = Form.useWatch('visibility', form)

  async function load() {
    setLoading(true)
    try {
      const res = await adminListContests({ page, size: pageSize })
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

  function openCreate() {
    setMetadataContest(null)
    form.resetFields()
    form.setFieldsValue({ rule: 'acm', visibility: 'public', rankboardVisible: true })
    setMetadataOpen(true)
  }

  function openEdit(contest: Contest) {
    setMetadataContest(contest)
    form.resetFields()
    form.setFieldsValue({
      ...contest,
      range: [dayjs(contest.beginAt), dayjs(contest.endAt)],
      freezeAt: contest.freezeAt ? dayjs(contest.freezeAt) : null,
      password: undefined,
    })
    setMetadataOpen(true)
  }

  async function handleMetadataSave() {
    const values = await form.validateFields()
    const payload = {
      title: values.title,
      description: values.description ?? '',
      rule: values.rule,
      beginAt: values.range[0].toISOString(),
      endAt: values.range[1].toISOString(),
      freezeAt: values.freezeAt?.toISOString(),
      visibility: values.visibility,
      password: values.visibility === 'password' ? values.password : undefined,
      rankboardVisible: values.rankboardVisible,
    }
    setSaving(true)
    try {
      if (metadataContest) {
        await adminUpdateContest(metadataContest.id, payload)
        message.success('比赛信息已更新')
        setMetadataOpen(false)
        await load()
      } else {
        const contest = await adminCreateContest(payload)
        message.success('比赛已创建，接下来请选择题目')
        setMetadataOpen(false)
        setEditingContest(contest)
        if (page === 1) await load()
        else setPage(1)
      }
    } catch (error: any) {
      message.error(error.response?.data?.error ?? (metadataContest ? '更新失败' : '创建失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card
      title="比赛管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          创建比赛
        </Button>
      }
    >
      <Table<Contest>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={{ current: page, pageSize, total, onChange: setPage, showSizeChanger: false }}
        columns={[
          { title: '标题', dataIndex: 'title', ellipsis: true },
          { title: '赛制', dataIndex: 'rule', width: 90, render: (rule: string) => <Tag>{rule === 'acm' ? 'ACM' : 'IOI'}</Tag> },
          { title: '可见性', dataIndex: 'visibility', width: 90, render: (value: Contest['visibility']) => <Tag>{contestVisibilityLabel(value)}</Tag> },
          { title: '开始', dataIndex: 'beginAt', width: 180, render: (time: string) => new Date(time).toLocaleString() },
          { title: '结束', dataIndex: 'endAt', width: 180, render: (time: string) => new Date(time).toLocaleString() },
          {
            title: '操作',
            width: 190,
            render: (_, contest) => (
              <Space>
                <Button size="small" onClick={() => openEdit(contest)}>编辑</Button>
                <Button size="small" type="primary" ghost onClick={() => setEditingContest(contest)}>管理题目</Button>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={metadataContest ? `编辑比赛 · ${metadataContest.title}` : '创建比赛'}
        open={metadataOpen}
        onOk={handleMetadataSave}
        confirmLoading={saving}
        onCancel={() => setMetadataOpen(false)}
        width={620}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入比赛名称' }]}>
            <Input placeholder="比赛名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} placeholder="比赛说明、规则或注意事项" />
          </Form.Item>
          <Space size="large" wrap>
            <Form.Item name="rule" label="赛制">
              <Select
                style={{ width: 130 }}
                options={[{ value: 'acm', label: 'ACM/ICPC' }]}
              />
            </Form.Item>
            <Form.Item name="visibility" label="可见性">
              <Select
                style={{ width: 130 }}
                options={[
                  { value: 'public', label: '公开' },
                  { value: 'private', label: '私有' },
                  { value: 'password', label: '密码赛' },
                ]}
              />
            </Form.Item>
            <Form.Item name="rankboardVisible" label="显示榜单" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Space>
          {formVisibility === 'password' && (
            <Form.Item
              name="password"
              label={metadataContest?.visibility === 'password' ? '新比赛密码（留空则不修改）' : '比赛密码'}
              rules={metadataContest?.visibility === 'password' ? [] : [{ required: true, message: '请输入比赛密码' }]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
          )}
          <Form.Item name="range" label="起止时间" rules={[{ required: true, message: '请选择比赛时间' }]}>
            <DatePicker.RangePicker showTime style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="freezeAt" label="封榜时间（可选）">
            <DatePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <ContestProblemManager
        contest={editingContest}
        onClose={() => setEditingContest(null)}
      />
    </Card>
  )
}

type ProblemChoice = Pick<Problem, 'id' | 'title' | 'difficulty' | 'visibility'>

function ContestProblemManager({ contest, onClose }: { contest: Contest | null; onClose: () => void }) {
  const [choices, setChoices] = useState<ProblemChoice[]>([])
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [addProblemId, setAddProblemId] = useState<string>()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!contest) return
    setLoading(true)
    Promise.all([adminListProblems({ page: 1, size: 100 }), adminGetContest(contest.id)])
      .then(([problemList, detail]) => {
        const allChoices: ProblemChoice[] = problemList.items.map(({ id, title, difficulty, visibility }) => ({
          id,
          title,
          difficulty,
          visibility,
        }))
        for (const problem of detail.problems) {
          if (!allChoices.some((item) => item.id === problem.problemId)) {
            allChoices.push({
              id: problem.problemId,
              title: problem.title,
              difficulty: problem.difficulty,
              visibility: problem.visibility,
            })
          }
        }
        setChoices(allChoices)
        setSelectedIds(detail.problems.map((problem) => problem.problemId))
        setAddProblemId(undefined)
      })
      .catch((error: any) => message.error(error.response?.data?.error ?? '比赛题目加载失败'))
      .finally(() => setLoading(false))
  }, [contest])

  const choiceMap = useMemo(() => new Map(choices.map((problem) => [problem.id, problem])), [choices])
  const availableOptions = choices
    .filter((problem) => !selectedIds.includes(problem.id))
    .map((problem) => ({ value: problem.id, label: `${problem.title}（难度 ${problem.difficulty}）` }))

  function moveProblem(index: number, delta: number) {
    const nextIndex = index + delta
    if (nextIndex < 0 || nextIndex >= selectedIds.length) return
    const next = [...selectedIds]
    ;[next[index], next[nextIndex]] = [next[nextIndex], next[index]]
    setSelectedIds(next)
  }

  async function save() {
    if (!contest) return
    setSaving(true)
    try {
      await adminSetContestProblems(contest.id, { problemIds: selectedIds })
      message.success(`已保存 ${selectedIds.length} 道比赛题目`)
      onClose()
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '比赛题目保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={contest ? `管理题目 · ${contest.title}` : '管理题目'}
      open={!!contest}
      onOk={save}
      confirmLoading={saving}
      okText="保存题目顺序"
      onCancel={onClose}
      width={720}
      destroyOnHidden
    >
      <Space.Compact style={{ width: '100%', marginBottom: 16 }}>
        <Select
          showSearch
          value={addProblemId}
          placeholder="搜索并选择要加入的题目"
          optionFilterProp="label"
          options={availableOptions}
          loading={loading}
          onChange={setAddProblemId}
          style={{ flex: 1 }}
        />
        <Button
          type="primary"
          disabled={!addProblemId}
          onClick={() => {
            if (!addProblemId) return
            setSelectedIds([...selectedIds, addProblemId])
            setAddProblemId(undefined)
          }}
        >
          加入比赛
        </Button>
      </Space.Compact>

      <List
        loading={loading}
        bordered
        dataSource={selectedIds}
        locale={{ emptyText: '尚未选择题目' }}
        renderItem={(problemId, index) => {
          const problem = choiceMap.get(problemId)
          return (
            <List.Item
              actions={[
                <Button key="up" type="text" icon={<ArrowUpOutlined />} disabled={index === 0} onClick={() => moveProblem(index, -1)} />,
                <Button key="down" type="text" icon={<ArrowDownOutlined />} disabled={index === selectedIds.length - 1} onClick={() => moveProblem(index, 1)} />,
                <Button key="delete" type="text" danger icon={<DeleteOutlined />} onClick={() => setSelectedIds(selectedIds.filter((id) => id !== problemId))} />,
              ]}
            >
              <List.Item.Meta
                avatar={<Tag color="blue">{String.fromCharCode(65 + index)}</Tag>}
                title={problem?.title ?? problemId}
                description={
                  <Space>
                    <Typography.Text type="secondary">难度 {problem?.difficulty ?? '—'}</Typography.Text>
                    <Tag>{visibilityLabel(problem?.visibility)}</Tag>
                  </Space>
                }
              />
            </List.Item>
          )
        }}
      />
    </Modal>
  )
}

function visibilityLabel(visibility?: Problem['visibility']) {
  if (visibility === 'public') return '公开'
  if (visibility === 'private') return '私有'
  return '草稿'
}

function contestVisibilityLabel(visibility: Contest['visibility']) {
  if (visibility === 'public') return '公开'
  if (visibility === 'password') return '密码赛'
  return '私有'
}
