import { Link } from 'react-router-dom'
import { Compass } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/misc'

export default function NotFoundPage() {
  return (
    <div className="py-20">
      <EmptyState
        icon={<Compass />}
        title="页面不存在"
        description="链接可能已经失效,或者地址输错了。"
        action={
          <Button asChild>
            <Link to="/">返回首页</Link>
          </Button>
        }
      />
    </div>
  )
}
