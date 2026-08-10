import { useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Code2 } from 'lucide-react'
import { useAuth } from '@/auth/AuthContext'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const toast = useToast()
  const { login, register } = useAuth()

  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // Send the user back to whatever they were trying to reach.
  const redirectTo = (location.state as { from?: string } | null)?.from ?? '/'

  async function handleSubmit(mode: 'login' | 'register') {
    if (!username.trim() || !password) {
      toast.warning('请填写用户名与密码')
      return
    }
    if (mode === 'register' && !email.trim()) {
      toast.warning('请填写邮箱')
      return
    }
    setSubmitting(true)
    try {
      if (mode === 'login') await login(username.trim(), password)
      else await register(username.trim(), email.trim(), password)
      toast.success(mode === 'login' ? '登录成功' : '注册成功')
      navigate(redirectTo, { replace: true })
    } catch (error) {
      toast.error(apiError(error, mode === 'login' ? '登录失败' : '注册失败'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-sm flex-col gap-6 px-4 py-16">
      <div className="flex flex-col items-center gap-2 text-center">
        <span className="grid size-10 place-items-center rounded-lg bg-primary text-primary-foreground">
          <Code2 className="size-5" />
        </span>
        <h1 className="text-lg font-semibold tracking-tight">
          Vertex<span className="text-primary">OJ</span>
        </h1>
      </div>

      <Card>
        <CardContent className="pt-5">
          <Tabs defaultValue="login" className="flex flex-col gap-4">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="login">登录</TabsTrigger>
              <TabsTrigger value="register">注册</TabsTrigger>
            </TabsList>

            <TabsContent value="login" className="flex flex-col gap-3">
              <Field label="用户名" id="login-username">
                <Input
                  id="login-username"
                  autoComplete="username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  onKeyDown={(event) => event.key === 'Enter' && handleSubmit('login')}
                />
              </Field>
              <Field label="密码" id="login-password">
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  onKeyDown={(event) => event.key === 'Enter' && handleSubmit('login')}
                />
              </Field>
              <Button className="mt-1" loading={submitting} onClick={() => handleSubmit('login')}>
                登录
              </Button>
            </TabsContent>

            <TabsContent value="register" className="flex flex-col gap-3">
              <Field label="用户名" id="register-username">
                <Input
                  id="register-username"
                  autoComplete="username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </Field>
              <Field label="邮箱" id="register-email">
                <Input
                  id="register-email"
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                />
              </Field>
              <Field label="密码" id="register-password">
                <Input
                  id="register-password"
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  onKeyDown={(event) => event.key === 'Enter' && handleSubmit('register')}
                />
              </Field>
              <Button className="mt-1" loading={submitting} onClick={() => handleSubmit('register')}>
                注册
              </Button>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <p className="text-center text-xs text-muted-foreground">
        不登录也可以{' '}
        <Link to="/problems" className="text-primary hover:underline">
          浏览题库
        </Link>
      </p>
    </div>
  )
}

function Field({ label, id, children }: { label: string; id: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}
