import { Outlet } from 'react-router-dom'
import { Code2, Trophy } from 'lucide-react'
import { NavLink } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import { cn } from '@/lib/utils'

const sections = [
  { to: '/workspace/problems', label: '题目', description: '出题与协作', icon: Code2 },
  { to: '/workspace/contests', label: '比赛', description: '编排与赛务', icon: Trophy },
]

export default function WorkspaceLayout() {
  const { domain } = useDomain()
  const name = domain.official ? 'Vertex' : domain.name
  return (
    <div className="page-shell grid items-start gap-7 lg:grid-cols-[180px_minmax(0,1fr)] lg:gap-10">
      <aside className="lg:sticky lg:top-24">
        <div className="mb-4 px-2">
          <p className="text-base font-semibold">工作台</p>
          <p className="mt-1 truncate text-xs text-muted-foreground" title={name}>
            {name}
          </p>
        </div>
        <nav
          aria-label="工作台分区"
          className="flex gap-1 border-b pb-3 lg:flex-col lg:border-b-0 lg:pb-0"
        >
          {sections.map(({ to, label, description, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                cn(
                  'flex flex-1 items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors',
                  isActive
                    ? 'bg-primary/8 font-medium text-primary'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                )
              }
            >
              <Icon className="size-4 shrink-0" />
              <span>
                <span className="block">{label}</span>
                <span className="mt-1 hidden text-xs font-normal text-muted-foreground lg:block">
                  {description}
                </span>
              </span>
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="min-w-0">
        <Outlet />
      </div>
    </div>
  )
}
