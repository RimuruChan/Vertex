import { Layout, Menu, Dropdown, Space, Avatar, Typography, message } from 'antd'
import { CodeOutlined, TrophyOutlined, HomeOutlined, BookOutlined, UserOutlined, LogoutOutlined, SettingOutlined } from '@ant-design/icons'
import type { MenuProps } from 'antd'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from './auth/AuthContext'

const { Header, Content, Footer } = Layout

// App 布局:顶部导航 + 内容区。
export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const { user, logout } = useAuth()

  const items: MenuProps['items'] = [
    { key: '/', icon: <HomeOutlined />, label: <Link to="/">首页</Link> },
    { key: '/problems', icon: <CodeOutlined />, label: <Link to="/problems">题库</Link> },
    { key: '/contests', icon: <TrophyOutlined />, label: <Link to="/contests">比赛</Link> },
    { key: '/submissions', icon: <BookOutlined />, label: <Link to="/submissions">提交记录</Link> },
  ]

  // admin 专属入口
  if (user?.role === 'admin') {
    items.push({
      key: 'admin',
      icon: <SettingOutlined />,
      label: '管理',
      children: [
        { key: '/admin/problems', label: <Link to="/admin/problems">题目管理</Link> },
        { key: '/admin/contests', label: <Link to="/admin/contests">比赛管理</Link> },
      ],
    })
  }

  const userMenu = user
    ? {
        items: [
          {
            key: 'logout',
            icon: <LogoutOutlined />,
            label: '退出登录',
            onClick: async () => {
              await logout().catch(() => undefined)
              message.success('已退出')
              navigate('/')
            },
          },
        ],
      }
    : undefined

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', alignItems: 'center' }}>
        <div style={{ color: '#fff', fontWeight: 700, fontSize: 18, marginRight: 32 }}>
          Vertex<span style={{ color: '#1677ff' }}>OJ</span>
        </div>
        <Menu
          theme="dark"
          mode="horizontal"
          selectedKeys={[location.pathname]}
          items={items}
          style={{ flex: 1, minWidth: 0 }}
        />
        <Space>
          {user ? (
            <Dropdown menu={userMenu}>
              <Space style={{ color: '#fff', cursor: 'pointer' }}>
                <Avatar size="small" icon={<UserOutlined />} />
                <Typography.Text style={{ color: '#fff' }}>{user.username}</Typography.Text>
              </Space>
            </Dropdown>
          ) : (
            <Link to="/login">
              <Typography.Text style={{ color: '#fff' }}>登录 / 注册</Typography.Text>
            </Link>
          )}
        </Space>
      </Header>
      <Content style={{ padding: '0 24px', marginTop: 24, maxWidth: 1200, width: '100%', margin: '24px auto' }}>
        <Outlet />
      </Content>
      <Footer style={{ textAlign: 'center', color: '#999' }}>
        Vertex Online Judge ©2026
      </Footer>
    </Layout>
  )
}
