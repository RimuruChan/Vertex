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

export default function MockMenu() {
  const confirm = useConfirm()
  const [verdict, setVerdict] = useState(mockAPI.nextVerdict)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="border-primary/20 bg-primary/5 text-primary"
          aria-label="演示模式设置"
        >
          <FlaskConical className="size-3.5" />
          <span className="hidden sm:inline">演示模式</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72 p-3">
        <DropdownMenuLabel className="px-0">本地交互演示</DropdownMenuLabel>
        <p className="mb-4 text-xs leading-5 text-muted-foreground">
          数据保存在当前浏览器。提交仅模拟评测，不执行代码，也不会连接后端。
        </p>
        <label className="mb-3 flex flex-col gap-1.5 text-xs">
          账号身份
          <select
            className="h-9 rounded-md border border-input bg-card px-2 text-sm"
            value={mockAPI.state.user?.id ?? 'guest'}
            onChange={(e) => changeMockIdentity(e.target.value)}
          >
            {mockIdentities.map((identity) => (
              <option key={identity.user?.id ?? 'guest'} value={identity.user?.id ?? 'guest'}>
                {identity.label}
              </option>
            ))}
          </select>
        </label>
        <label className="mb-3 flex flex-col gap-1.5 text-xs">
          页面状态
          <select
            className="h-9 rounded-md border border-input bg-card px-2 text-sm"
            value={mockAPI.scenario}
            onChange={(e) => changeScenario(e.target.value as MockScenario)}
          >
            <option value="normal">正常加载</option>
            <option value="slow">慢速加载</option>
            <option value="empty">空列表</option>
            <option value="error">加载失败</option>
          </select>
        </label>
        <label className="mb-3 flex flex-col gap-1.5 text-xs">
          下一次模拟评测结果
          <select
            className="h-9 rounded-md border border-input bg-card px-2 text-sm"
            value={verdict}
            onChange={(e) => {
              mockAPI.nextVerdict = e.target.value
              setVerdict(e.target.value)
              persistMock()
            }}
          >
            <option value="Accepted">AC · 通过</option>
            <option value="Wrong Answer">WA · 答案错误</option>
            <option value="Time Limit Exceeded">TLE · 超时</option>
            <option value="Compile Error">CE · 编译错误</option>
          </select>
        </label>
        <p className="text-xs text-muted-foreground">
          演示账号的统一密码 <span className="font-mono">demo123</span>
        </p>
        <DropdownMenuSeparator />
        <Button
          variant="ghost"
          size="sm"
          className="w-full"
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
