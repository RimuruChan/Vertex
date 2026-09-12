import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useState } from 'react'
import { FlaskConical, RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { changeMockIdentity, changeScenario, mockAPI, persistMock, resetMock } from './adapter'
import { mockIdentities } from './identities'
import type { MockScenario } from './api'
import { cn } from '@/lib/utils'
import { useDomainSlug } from '@/domain/navigation'
import { useOptionalDomain } from '@/domain/DomainContext'
import { useToast } from '@/components/ui/toast'
import { addAnnouncementExamples } from './announcement-examples'

export default function MockMenu({
  placement = 'floating',
}: {
  placement?: 'footer' | 'floating'
}) {
  const confirm = useConfirm()
  const slug = useDomainSlug(),
    domain = useOptionalDomain(),
    toast = useToast()
  const [verdict, setVerdict] = useState(mockAPI.nextVerdict)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant={placement === 'footer' ? 'ghost' : 'outline'}
          size="sm"
          className={cn(
            'text-muted-foreground hover:text-primary',
            placement === 'floating' &&
              'fixed bottom-[max(1rem,env(safe-area-inset-bottom))] right-4 z-40 h-9 rounded-full border-primary/20 bg-card px-3 shadow-md',
          )}
          aria-label="演示模式设置"
        >
          <FlaskConical className="size-3.5" />
          <span className={placement === 'floating' ? 'hidden sm:inline' : undefined}>
            演示设置
          </span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="top"
        align="end"
        sideOffset={10}
        collisionPadding={16}
        className="max-h-[calc(100dvh-5rem)] w-72 overflow-y-auto p-3"
      >
        <DropdownMenuLabel className="px-0">本地交互演示</DropdownMenuLabel>
        <p className="mb-4 text-xs leading-5 text-muted-foreground">
          数据保存在当前浏览器。提交仅模拟评测，不执行代码，也不会连接后端。
        </p>
        <label className="mb-3 flex flex-col gap-1.5 text-xs">
          账号身份
          <Select
            value={mockAPI.state.user?.id ?? 'guest'}
            onValueChange={(value) => changeMockIdentity(value)}
          >
            <SelectTrigger aria-label="账号身份" className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {mockIdentities.map((identity) => (
                <SelectItem key={identity.user?.id ?? 'guest'} value={identity.user?.id ?? 'guest'}>
                  {identity.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </label>
        <label className="mb-3 flex flex-col gap-1.5 text-xs">
          页面状态
          <Select
            value={mockAPI.scenario}
            onValueChange={(value) => changeScenario(value as MockScenario)}
          >
            <SelectTrigger aria-label="页面状态" className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="normal">正常加载</SelectItem>
              <SelectItem value="slow">慢速加载</SelectItem>
              <SelectItem value="empty">空列表</SelectItem>
              <SelectItem value="error">加载失败</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <label className="mb-3 flex flex-col gap-1.5 text-xs">
          下一次模拟评测结果
          <Select
            value={verdict}
            onValueChange={(value) => {
              mockAPI.nextVerdict = value
              setVerdict(value)
              persistMock()
            }}
          >
            <SelectTrigger aria-label="下一次模拟评测结果" className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="Accepted">AC · 通过</SelectItem>
              <SelectItem value="Wrong Answer">WA · 答案错误</SelectItem>
              <SelectItem value="Time Limit Exceeded">TLE · 超时</SelectItem>
              <SelectItem value="Compile Error">CE · 编译错误</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <p className="text-xs text-muted-foreground">
          演示账号的统一密码 <span className="font-mono">demo123</span>
        </p>
        <DropdownMenuSeparator />
        {(domain?.can('domain.resources.manage') ?? mockAPI.state.user?.role === 'admin') && (
          <Button
            variant="outline"
            size="sm"
            className="w-full"
            onClick={() => {
              try {
                addAnnouncementExamples(mockAPI, slug)
                changeScenario(mockAPI.scenario)
              } catch (error) {
                toast.error((error as Error).message)
              }
            }}
          >
            添加公告示例
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          className="mt-2 w-full text-destructive hover:border-destructive/60 hover:bg-destructive/5 hover:text-destructive"
          onClick={async () => {
            if (
              await confirm({
                title: '重置演示数据？',
                description: '清除本地模拟提交、题解、讨论与演示代码草稿，恢复初始数据。',
                confirmLabel: '重置',
              })
            )
              resetMock()
          }}
        >
          <RotateCcw />
          重置演示数据
        </Button>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
