import { Card, Button, Space, Typography, Row, Col } from 'antd'
import { Link } from 'react-router-dom'
import { CodeOutlined, TrophyOutlined, EditOutlined, MessageOutlined } from '@ant-design/icons'
import { currentUser } from '../api/client'

const { Title, Paragraph } = Typography

export default function HomePage() {
  const user = currentUser()

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Card style={{ background: 'linear-gradient(135deg,#1677ff 0%,#0958d9 100%)', color: '#fff' }}>
        <Title level={2} style={{ color: '#fff', marginBottom: 8 }}>
          Vertex Online Judge
        </Title>
        <Paragraph style={{ color: 'rgba(255,255,255,0.85)', fontSize: 16 }}>
          一个自托管的在线判题平台 — 题目、比赛、题解与交流
        </Paragraph>
        {user ? (
          <Space>
            <Link to="/problems">
              <Button type="primary" size="large" icon={<CodeOutlined />}>
                进入题库
              </Button>
            </Link>
            <Link to="/contests">
              <Button size="large" ghost icon={<TrophyOutlined />}>
                浏览比赛
              </Button>
            </Link>
          </Space>
        ) : (
          <Space>
            <Link to="/login">
              <Button type="primary" size="large">
                登录 / 注册
              </Button>
            </Link>
          </Space>
        )}
      </Card>

      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <CodeOutlined style={{ fontSize: 28, color: '#1677ff' }} />
            <Title level={4}>在线判题</Title>
            <Typography.Text type="secondary">
              支持 C / C++ / Python,隔离沙箱评测,逐测试点反馈
            </Typography.Text>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <TrophyOutlined style={{ fontSize: 28, color: '#faad14' }} />
            <Title level={4}>比赛</Title>
            <Typography.Text type="secondary">
              ACM/ICPC 赛制,实时榜单,封榜与解榜
            </Typography.Text>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <EditOutlined style={{ fontSize: 28, color: '#52c41a' }} />
            <Title level={4}>出题</Title>
            <Typography.Text type="secondary">
              管理员出题,测试数据管理,题目可见性控制
            </Typography.Text>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <MessageOutlined style={{ fontSize: 28, color: '#eb2f96' }} />
            <Title level={4}>交流</Title>
            <Typography.Text type="secondary">
              题目评论与题解,Markdown + LaTeX 渲染
            </Typography.Text>
          </Card>
        </Col>
      </Row>
    </Space>
  )
}
