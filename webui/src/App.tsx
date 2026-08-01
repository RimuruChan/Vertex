import { Layout, Menu } from 'antd'
import { CodeOutlined, TrophyOutlined, HomeOutlined, BookOutlined } from '@ant-design/icons'
import { Link, Outlet, useLocation } from 'react-router-dom'

const { Header, Content, Footer } = Layout

// App 布局:顶部导航 + 内容区。
export default function App() {
  const location = useLocation()

  const items = [
    { key: '/', icon: <HomeOutlined />, label: <Link to="/">首页</Link> },
    { key: '/problems', icon: <CodeOutlined />, label: <Link to="/problems">题库</Link> },
    { key: '/contests', icon: <TrophyOutlined />, label: <Link to="/contests">比赛</Link> },
    { key: '/submissions', icon: <BookOutlined />, label: <Link to="/submissions">提交记录</Link> },
  ]

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
