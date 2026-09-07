import { useState } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import { useAuth } from '@/auth/AuthContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PageSpinner } from '@/components/ui/misc'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const toast = useToast()
  const { user, ready, login, register } = useAuth()

  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // Send the user back to whatever they were trying to reach.
  const requestedRedirect = (location.state as { from?: string } | null)?.from
  const redirectTo =
    requestedRedirect?.startsWith('/') &&
    !requestedRedirect.startsWith('//') &&
    !requestedRedirect.startsWith('/login')
      ? requestedRedirect
      : '/'

  if (!ready) return <PageSpinner />
  if (user) return <Navigate to={redirectTo} replace />

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
    <div className="mx-auto flex w-full max-w-md flex-col gap-7 px-4 py-10 sm:py-16">
      <header>
        <p className="eyebrow">欢迎回来</p>
        <h1 className="text-3xl font-semibold tracking-tight">继续你的练习</h1>
        <p className="mt-2 text-sm text-muted-foreground">登录或创建一个新账号。</p>
      </header>

      <div className="surface-panel p-6 sm:p-7">
        {import.meta.env.VITE_MOCK === 'true' ? (
          <div className="mb-5 rounded-lg bg-primary/5 p-3 text-xs leading-5 text-muted-foreground">
            <p>本地演示无需真实账号，请勿输入个人密码。</p>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              className="mt-2 w-full"
              onClick={() => {
                setUsername('demo')
                setPassword('demo123')
              }}
            >
              填入演示账号
            </Button>
          </div>
        ) : null}
        <Tabs defaultValue="login" className="flex flex-col gap-4">
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="login">登录</TabsTrigger>
            <TabsTrigger value="register">注册</TabsTrigger>
          </TabsList>

          <TabsContent value="login" asChild>
            <form
              className="flex flex-col gap-3"
              onSubmit={(event) => {
                event.preventDefault()
                void handleSubmit('login')
              }}
            >
              <Field label="用户名" id="login-username">
                <Input
                  id="login-username"
                  autoComplete="username"
                  minLength={3}
                  maxLength={32}
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  required
                />
              </Field>
              <Field label="密码" id="login-password">
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="current-password"
                  minLength={6}
                  maxLength={72}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
              </Field>
              <Button type="submit" className="mt-1" loading={submitting}>
                登录
              </Button>
            </form>
          </TabsContent>

          <TabsContent value="register" asChild>
            <form
              className="flex flex-col gap-3"
              onSubmit={(event) => {
                event.preventDefault()
                void handleSubmit('register')
              }}
            >
              <Field label="用户名" id="register-username">
                <Input
                  id="register-username"
                  autoComplete="username"
                  minLength={3}
                  maxLength={32}
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  required
                />
              </Field>
              <Field label="邮箱" id="register-email">
                <Input
                  id="register-email"
                  type="email"
                  autoComplete="email"
                  maxLength={254}
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  required
                />
              </Field>
              <Field label="密码" id="register-password">
                <Input
                  id="register-password"
                  type="password"
                  autoComplete="new-password"
                  minLength={6}
                  maxLength={72}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
              </Field>
              <Button type="submit" className="mt-1" loading={submitting}>
                注册
              </Button>
            </form>
          </TabsContent>
        </Tabs>
      </div>

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
