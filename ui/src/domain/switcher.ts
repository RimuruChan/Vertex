import type { DtoDomainResponse, DtoUserResponse } from '@/generated/api/model'
import { domainRoleLabel } from './labels'

export function switchableDomains(items: DtoDomainResponse[], currentSlug?: string) {
  const priority = (domain: DtoDomainResponse) => {
    if (domain.slug === currentSlug) return 0
    if (domain.archived) return 4
    if (domain.official) return 1
    return domain.memberStatus === 'active' ? 2 : 3
  }
  return items.filter((domain) => domain.canEnter).sort((a, b) => priority(a) - priority(b))
}

export function domainIdentityLabel(domain: DtoDomainResponse, user: DtoUserResponse | null) {
  if (!user) return '访客'
  if (user.role === 'admin') return '站点维护'
  if (domain.memberStatus === 'active') {
    return domain.ownerId === user.id ? '域 owner' : domainRoleLabel(domain.memberRole)
  }
  return (
    { invited: '待接受邀请', pending: '申请审核中', suspended: '成员资格已停用' }[
      domain.memberStatus
    ] || '未加入'
  )
}
