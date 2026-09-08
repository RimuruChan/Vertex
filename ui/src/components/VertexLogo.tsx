import { Triangle } from 'lucide-react'
import { cn } from '@/lib/utils'

export default function VertexLogo({ className }: { className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2 text-foreground', className)}>
      <Triangle
        className="size-5 shrink-0 fill-primary/10 text-primary"
        strokeWidth={2.5}
        aria-hidden="true"
      />
      <span className="text-lg font-semibold tracking-tight">vertex</span>
    </span>
  )
}
