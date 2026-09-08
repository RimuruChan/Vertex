import type { DomainPermission, DtoDomainResponse } from '@/generated/api/model'

export const domainSettingsSections: {
  path: string
  label: string
  permission: DomainPermission
}[] = [
  { path: '/settings', label: '概览与设置', permission: 'domain.settings.manage' },
  { path: '/settings/members', label: '成员', permission: 'domain.members.manage' },
  { path: '/settings/roles', label: '角色', permission: 'domain.roles.manage' },
  { path: '/settings/tags', label: '标签', permission: 'domain.resources.manage' },
  { path: '/settings/announcements', label: '公告', permission: 'domain.resources.manage' },
]

export function visibleDomainSettings(
  domain: Pick<DtoDomainResponse, 'permissions' | 'canArchive' | 'canTransfer'>,
) {
  return domainSettingsSections.filter(
    (item) =>
      domain.permissions.includes(item.permission) ||
      (item.path === '/settings' && (domain.canArchive || domain.canTransfer)),
  )
}
