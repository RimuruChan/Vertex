import { useState } from 'react'
import { Card, Form, Input, Button, Tabs, message, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { login, register } from '../api/client'

const { Title } = Typography

// LoginPage:登录 / 注册双 Tab。
export default function LoginPage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)

  async function handleLogin(values: { username: string; password: string }) {
    setLoading(true)
    try {
      await login(values.username, values.password)
      message.success('登录成功')
      navigate('/')
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '登录失败')
    } finally {
      setLoading(false)
    }
  }

  async function handleRegister(values: { username: string; email: string; password: string }) {
    setLoading(true)
    try {
      await register(values.username, values.email, values.password)
      message.success('注册成功,已自动登录')
      navigate('/')
    } catch (e: any) {
      message.error(e.response?.data?.error ?? '注册失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ maxWidth: 400, margin: '40px auto' }}>
      <Card>
        <Title level={3} style={{ textAlign: 'center' }}>
          Vertex OJ
        </Title>
        <Tabs
          items={[
            {
              key: 'login',
              label: '登录',
              children: (
                <Form onFinish={handleLogin} layout="vertical">
                  <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                    <Input placeholder="用户名" autoComplete="username" />
                  </Form.Item>
                  <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                    <Input.Password placeholder="密码" autoComplete="current-password" />
                  </Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    登录
                  </Button>
                </Form>
              ),
            },
            {
              key: 'register',
              label: '注册',
              children: (
                <Form onFinish={handleRegister} layout="vertical">
                  <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                    <Input placeholder="用户名(3-32位字母数字下划线)" />
                  </Form.Item>
                  <Form.Item name="email" rules={[{ required: true, type: 'email', message: '请输入邮箱' }]}>
                    <Input placeholder="邮箱" />
                  </Form.Item>
                  <Form.Item name="password" rules={[{ required: true, min: 6, message: '密码至少 6 位' }]}>
                    <Input.Password placeholder="密码(至少 6 位)" autoComplete="new-password" />
                  </Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    注册
                  </Button>
                </Form>
              ),
            },
          ]}
        />
      </Card>
    </div>
  )
}
