import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, InputNumber, message, Modal, Select, Space, Table, Tabs, Tag, Typography, Upload } from 'antd'
import { EyeOutlined, PlusOutlined, UploadOutlined } from '@ant-design/icons'
import { Link } from 'react-router-dom'
import {
  deleteApiAdminProblemsId as adminDeleteProblem,
  getApiAdminProblems as adminListProblems,
  postApiAdminProblems as adminCreateProblem,
  postApiAdminProblemsIdTestdata as adminUploadTestdata,
  putApiAdminProblemsId as adminUpdateProblem,
} from '../../generated/api/vertex'
import type { DtoProblemResponse as Problem } from '../../generated/api/model'
import MdRenderer from '../../components/MdRenderer'

const pageSize = 20

export default function AdminProblemPage() {
  const [data, setData] = useState<Problem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Problem | null>(null)
  const [keyword, setKeyword] = useState('')
  const [visibility, setVisibility] = useState<string>()
  const [activeTab, setActiveTab] = useState('edit')
  const [form] = Form.useForm()
  const statement = Form.useWatch('statementMd', form) ?? ''

  async function load() {
    setLoading(true)
    try {
      const res = await adminListProblems({
        page,
        size: pageSize,
        keyword: keyword || undefined,
        visibility,
      })
      setData(res.items)
      setTotal(res.total)
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '题目列表加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [page, keyword, visibility])

  function openCreate() {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({
      timeLimitMs: 1000,
      memoryLimitKb: 262144,
      difficulty: 1,
      visibility: 'draft',
      tags: [],
    })
    setActiveTab('edit')
    setModalOpen(true)
  }

  function openEdit(problem: Problem) {
    setEditing(problem)
    form.setFieldsValue(problem)
    setActiveTab('edit')
    setModalOpen(true)
  }

  async function handleSave() {
    const values = await form.validateFields()
    setSaving(true)
    try {
      if (editing) {
        await adminUpdateProblem(editing.id, values)
        message.success('题目已更新')
      } else {
        await adminCreateProblem(values)
        message.success('题目已创建，请继续上传测试数据')
      }
      setModalOpen(false)
      await load()
    } catch (error: any) {
      message.error(error.response?.data?.error ?? '保存失败')
    } finally {
      setSaving(false)
    }
  }

  function handleDelete(id: string) {
    Modal.confirm({
      title: '确认删除该题？',
      content: '题目、测试数据和相关记录将被删除，此操作不可撤销。',
      okType: 'danger',
      onOk: async () => {
        try {
          await adminDeleteProblem(id)
          message.success('已删除')
          await load()
        } catch (error: any) {
          message.error(error.response?.data?.error ?? '删除失败')
        }
      },
    })
  }

  return (
    <Card
      title="出题管理"
      extra={
        <Space wrap>
          <Input.Search
            allowClear
            placeholder="搜索标题或来源"
            style={{ width: 220 }}
            onSearch={(value) => {
              setPage(1)
              setKeyword(value.trim())
            }}
          />
          <Select
            allowClear
            placeholder="全部可见性"
            style={{ width: 130 }}
            options={[
              { value: 'draft', label: '草稿' },
              { value: 'private', label: '私有' },
              { value: 'public', label: '公开' },
            ]}
            onChange={(value) => {
              setPage(1)
              setVisibility(value)
            }}
          />
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建题目
          </Button>
        </Space>
      }
    >
      <Table<Problem>
        rowKey="id"
        loading={loading}
        dataSource={data}
        pagination={{ current: page, pageSize, total, onChange: setPage, showSizeChanger: false }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 90, render: (id: string) => <Tag>#{id.slice(0, 8)}</Tag> },
          {
            title: '标题',
            dataIndex: 'title',
            ellipsis: true,
            render: (title: string, problem) =>
              problem.visibility === 'public' ? <Link to={`/problems/${problem.id}`}>{title}</Link> : title,
          },
          { title: '来源', dataIndex: 'source', width: 130, ellipsis: true, render: (source: string) => source || '—' },
          {
            title: '可见性',
            dataIndex: 'visibility',
            width: 100,
            render: (value: Problem['visibility']) => (
              <Tag color={value === 'public' ? 'green' : value === 'private' ? 'orange' : 'default'}>
                {visibilityLabel(value)}
              </Tag>
            ),
          },
          { title: '难度', dataIndex: 'difficulty', width: 70, align: 'center' },
          { title: '提交', dataIndex: 'submissionCount', width: 70, align: 'center' },
          { title: '通过', dataIndex: 'acceptedCount', width: 70, align: 'center' },
          {
            title: '操作',
            width: 290,
            render: (_, problem) => (
              <Space>
                <Button size="small" onClick={() => openEdit(problem)}>编辑</Button>
                {problem.visibility === 'public' && (
                  <Link to={`/problems/${problem.id}`}>
                    <Button size="small" icon={<EyeOutlined />}>查看</Button>
                  </Link>
                )}
                <TestdataUploader problemId={problem.id} />
                <Button size="small" danger onClick={() => handleDelete(problem.id)}>删除</Button>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={editing ? '编辑题目' : '新建题目'}
        open={modalOpen}
        onOk={handleSave}
        confirmLoading={saving}
        onCancel={() => setModalOpen(false)}
        width={860}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'edit',
                label: '编辑',
                children: (
                  <>
                    <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入题目标题' }]}>
                      <Input placeholder="题目名称" />
                    </Form.Item>
                    <Form.Item name="source" label="来源">
                      <Input placeholder="例如：原创、Codeforces Round 123" />
                    </Form.Item>
                    <Form.Item name="statementMd" label="题面（Markdown + LaTeX）" rules={[{ required: true, message: '请输入题面' }]}>
                      <Input.TextArea
                        rows={12}
                        placeholder="请包含题目描述、输入格式、输出格式和样例；支持 $\\sum$ 等 LaTeX 公式"
                      />
                    </Form.Item>
                    <Space size="large" wrap>
                      <Form.Item name="difficulty" label="难度" rules={[{ required: true }]}>
                        <InputNumber min={1} max={10} />
                      </Form.Item>
                      <Form.Item name="timeLimitMs" label="时间限制（ms）" rules={[{ required: true }]}>
                        <InputNumber min={100} max={60000} step={100} />
                      </Form.Item>
                      <Form.Item name="memoryLimitKb" label="内存限制（KB）" rules={[{ required: true }]}>
                        <InputNumber min={16384} max={4194304} step={65536} />
                      </Form.Item>
                      <Form.Item name="visibility" label="可见性" rules={[{ required: true }]}>
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
                  </>
                ),
              },
              {
                key: 'preview',
                label: '题面预览',
                children: statement ? (
                  <Card size="small" className="markdown-body">
                    <MdRenderer content={statement} />
                  </Card>
                ) : (
                  <Typography.Text type="secondary">填写题面后可在这里预览。</Typography.Text>
                ),
              },
            ]}
          />
        </Form>
      </Modal>
    </Card>
  )
}

function TestdataUploader({ problemId }: { problemId: string }) {
  const [uploading, setUploading] = useState(false)

  return (
    <Upload
      accept=".zip"
      showUploadList={false}
      customRequest={async ({ file, onSuccess, onError }) => {
        setUploading(true)
        try {
          const result = await adminUploadTestdata(problemId, { file: file as File, checker: 'diff' })
          message.success(`上传成功：${result.caseCount} 个测试点`)
          onSuccess?.(result)
        } catch (error: any) {
          message.error(error.response?.data?.error ?? '上传失败')
          onError?.(error)
        } finally {
          setUploading(false)
        }
      }}
    >
      <Button size="small" loading={uploading} icon={<UploadOutlined />}>数据</Button>
    </Upload>
  )
}

function visibilityLabel(visibility: Problem['visibility']) {
  if (visibility === 'public') return '公开'
  if (visibility === 'private') return '私有'
  return '草稿'
}
