import { Outlet } from 'react-router-dom'
import { NavLink } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import PageHeading from '@/components/PageHeading'
import { visibleDomainSettings } from '@/domain/settings-navigation'

export default function DomainSettingsLayout() {
  const { domain } = useDomain()
  return (
    <div className="page-shell space-y-6">
      <PageHeading
        eyebrow="域 / 管理"
        title={`${domain.official ? 'Vertex' : domain.name} · 域设置`}
        description="设置、成员与权限都只作用于这个域。"
      />
      <nav aria-label="域设置分区" className="flex flex-wrap gap-2 border-b pb-3">
        {visibleDomainSettings(domain).map(({ path, label }) => (
          <NavLink
            key={path}
            to={path}
            end={path === '/settings'}
            className={({ isActive }) =>
              `rounded-md px-3 py-2 text-sm ${isActive ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:bg-muted'}`
            }
          >
            {label}
          </NavLink>
        ))}
      </nav>
      <Outlet />
    </div>
  )
}
