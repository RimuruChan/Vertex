import { useEffect, useState } from 'react'
import { Card, Table, Button, Modal, Form, Input, InputNumber, Select, Space, message, Tag, Upload } from 'antd'
import { UploadOutlined, PlusOutlined } from '@ant-design/icons'
import { adminListProblems, adminCreateProblem, adminUpdateProblem, adminDeleteProblem, adminUploadTestdata } from '../../api/client'
import type { Problem } from '../../api/types'

// AdminProblemPage:出题管理后台(题目 CRUD + 测试数据上传)。
export default function AdminProblemPage() {
  const [data, setData] = useState<Problem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Problem | null>(null)
  const [form] = Form.useForm()

  async function load() {
    setLoading(true)
    try {
      const res = await adminListProblems({ page, size: 20 })
      setData(res.items)
      setTotal(res.total)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page])

  function openCreate() {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ timeLimitMs: 1000, memoryLimitKb: 262144, difficulty: 1, visibility: 'draft' })
    setModalOpen(true)
  }

  function openEdit(p: Problem) {
    setEditing(p)
    form.setFieldsValue(p)
    setModalOpen(true)
  }

  async function handleSave() {
    const values = await form.validateFields()
    try {
      if (editing) {
        await adminUpdateProblem(editing.id, values)
        message.success('已更新')
      } else {
        await adminCreateProblem(values)
        message.success('已创建')
      }
      setModalOpen(false)
      load()
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '保存失败')
    }
  }

  async function handleDelete(id: string) {
    Modal.confirm({
      title: '确认删除该题?',
      okType: 'danger',
      onOk: async () => {
        await adminDeleteProblem(id)
        message.success('已删除')
        load()
      },
    })
  }

  return (
    <Card
      title="出题管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新建题目
        </Button>
      }
    >
      <Table<Problem>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={{ current: page, pageSize: 20, total, onChange: setPage }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 90, render: (id: string) => <Tag>#{id.slice(0, 8)}</Tag> },
          { title: '标题', dataIndex: 'title', ellipsis: true },
          {
            title: '可见性',
            dataIndex: 'visibility',
            width: 100,
            render: (v: string) => (
              <Tag color={v === 'public' ? 'green' : v === 'private' ? 'orange' : 'default'}>
                {v === 'public' ? '公开' : v === 'private' ? '私有' : '草稿'}
              </Tag>
            ),
          },
          { title: '难度', dataIndex: 'difficulty', width: 70, align: 'center' },
          { title: '提交', dataIndex: 'submissionCount', width: 80, align: 'center' },
          { title: '通过', dataIndex: 'acceptedCount', width: 80, align: 'center' },
          {
            title: '操作',
            width: 240,
            render: (_, p) => (
              <Space>
                <Button size="small" onClick={() => openEdit(p)}>
                  编辑
                </Button>
                <TestdataUploader problemId={p.id} />
                <Button size="small" danger onClick={() => handleDelete(p.id)}>
                  删除
                </Button>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={editing ? '编辑题目' : '新建题目'}
        open={modalOpen}
        onOk={handleSave}
        onCancel={() => setModalOpen(false)}
        width={640}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input placeholder="题目名称" />
          </Form.Item>
          <Form.Item name="statementMd" label="题面(Markdown + LaTeX)">
            <Input.TextArea rows={8} placeholder="支持 $\\sum$ 等 LaTeX 公式" />
          </Form.Item>
          <Space size="large">
            <Form.Item name="difficulty" label="难度" rules={[{ required: true }]}>
              <InputNumber min={1} max={10} />
            </Form.Item>
            <Form.Item name="timeLimitMs" label="时间限制(ms)">
              <InputNumber min={100} step={100} />
            </Form.Item>
            <Form.Item name="memoryLimitKb" label="内存限制(KB)">
              <InputNumber min={65536} step={65536} />
            </Form.Item>
            <Form.Item name="visibility" label="可见性">
              <Select
                style={{ width: 110 }}
                options={[
                  { value: 'draft', label: '草稿' },
                  { value: 'private', label: '私有' },
                  { value: 'public', label: '公开' },
                ]}
              />
            </Form.Item>
          </Space>
          <Form.Item name="tags" label="标签">
            <Select mode="tags" placeholder="输入标签后回车" open={false} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

// TestdataUploader:上传测试数据 zip(1.in/1.out...)。
function TestdataUploader({ problemId }: { problemId: string }) {
  const [uploading, setUploading] = useState(false)

  return (
    <Upload
      accept=".zip"
      showUploadList={false}
      customRequest={async ({ file, onSuccess, onError }) => {
        setUploading(true)
        try {
          const res = await adminUploadTestdata(problemId, file as File, 'diff')
          message.success(`上传成功:${res.caseCount} 个测试点`)
          onSuccess?.(res)
        } catch (e: any) {
          message.error(e.response?.data?.error ?? '上传失败')
          onError?.(e)
        } finally {
          setUploading(false)
        }
      }}
    >
      <Button size="small" loading={uploading} icon={<UploadOutlined />}>
        数据
      </Button>
    </Upload>
  )
}
