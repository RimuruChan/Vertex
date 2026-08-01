import { useEffect, useState } from 'react'
import { Card, Button, Modal, Form, Input, DatePicker, Select, Space, message, Tag, Table } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { adminCreateContest, listContests } from '../../api/client'
import type { Contest } from '../../api/types'
import { useNavigate } from 'react-router-dom'

// AdminContestPage:比赛管理(创建比赛 + 设置题目)。
export default function AdminContestPage() {
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const [data, setData] = useState<Contest[]>([])
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm()

  async function load() {
    setLoading(true)
    try {
      const res = await listContests()
      setData(res.items)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function handleCreate() {
    const values = await form.validateFields()
    const payload = {
      title: values.title,
      description: values.description ?? '',
      rule: values.rule ?? 'acm',
      beginAt: values.range[0].toISOString(),
      endAt: values.range[1].toISOString(),
      visibility: values.visibility ?? 'public',
      password: values.password ?? '',
      rankboardVisible: true,
    }
    try {
      await adminCreateContest(payload)
      message.success('比赛已创建')
      setCreateOpen(false)
      load()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '创建失败')
    }
  }

  return (
    <Card
      title="比赛管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          创建比赛
        </Button>
      }
    >
      <Table<Contest>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={false}
        columns={[
          { title: '标题', dataIndex: 'title', ellipsis: true },
          { title: '赛制', dataIndex: 'rule', width: 90, render: (r: string) => <Tag>{r === 'acm' ? 'ACM' : 'IOI'}</Tag> },
          { title: '开始', dataIndex: 'beginAt', width: 180, render: (t: string) => new Date(t).toLocaleString() },
          {
            title: '操作',
            width: 140,
            render: (_, c) => (
              <Button size="small" onClick={() => navigate(`/contests/${c.id}`)}>
                管理题目
              </Button>
            ),
          },
        ]}
      />

      <Modal
        title="创建比赛"
        open={createOpen}
        onOk={handleCreate}
        onCancel={() => setCreateOpen(false)}
        width={560}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input placeholder="比赛名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
          <Space size="large">
            <Form.Item name="rule" label="赛制" initialValue="acm">
              <Select
                style={{ width: 120 }}
                options={[
                  { value: 'acm', label: 'ACM/ICPC' },
                  { value: 'ioi', label: 'IOI' },
                ]}
              />
            </Form.Item>
            <Form.Item name="visibility" label="可见性" initialValue="public">
              <Select
                style={{ width: 120 }}
                options={[
                  { value: 'public', label: '公开' },
                  { value: 'private', label: '私有' },
                ]}
              />
            </Form.Item>
          </Space>
          <Form.Item name="range" label="起止时间" rules={[{ required: true }]}>
            <DatePicker.RangePicker showTime />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
