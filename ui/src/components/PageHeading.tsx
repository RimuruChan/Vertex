import type { ReactNode } from 'react'

export default function PageHeading({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow: string
  title: string
  description?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="mb-7 flex flex-wrap items-end justify-between gap-5 sm:mb-8">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1 className="text-[1.75rem] font-semibold leading-tight tracking-tight sm:text-[2rem]">
          {title}
        </h1>
        {description ? (
          <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  )
}
