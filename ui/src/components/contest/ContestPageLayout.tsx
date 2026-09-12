import type { ComponentProps, ReactNode } from 'react'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'

export function ContestPageHeader({
  title,
  description,
  action,
}: {
  title: string
  description: ReactNode
  action?: ReactNode
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 py-1">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold leading-8 tracking-tight">{title}</h1>
        <div className="mt-2 text-sm leading-6 text-muted-foreground">{description}</div>
      </div>
      {action}
    </header>
  )
}

export function ContestPanel({ className, ...props }: ComponentProps<typeof Card>) {
  return <Card className={cn('min-w-0 rounded-xl', className)} {...props} />
}
