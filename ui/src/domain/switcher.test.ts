import { describe, expect, it } from 'vitest'
import type { DtoDomainResponse } from '@/generated/api/model'
import { domainIdentityLabel, switchableDomains } from './switcher'

const domain: DtoDomainResponse = {
  id: 'training-id',
  slug: 'training',
  name: '训练域',
  description: '',
  ownerId: 'owner-id',
  ownerName: 'owner',
  official: false,
  archived: false,
  canArchive: false,
  canEnter: true,
  canTransfer: false,
  createdAt: '',
  memberRole: 'author',
  memberStatus: 'active',
  permissions: [],
  visibility: 'public',
  joinPolicy: 'open',
}
const member = { id: 'member-id', username: 'member', email: '', role: 'user' }

describe('domain switcher presentation', () => {
  it('pins the current domain, then official and memberships, without changing the source list', () => {
    const items = [
      { ...domain, slug: 'archived', archived: true },
      { ...domain, slug: 'public', memberStatus: '' },
      { ...domain, slug: 'official', official: true },
      domain,
      { ...domain, slug: 'member' },
      { ...domain, slug: 'blocked', canEnter: false },
    ]
    const before = structuredClone(items)
    expect(switchableDomains(items, 'training').map((item) => item.slug)).toEqual([
      'training',
      'official',
      'member',
      'public',
      'archived',
    ])
    expect(items).toEqual(before)
    expect(switchableDomains(items, 'archived')[0].slug).toBe('archived')
    expect(switchableDomains(items)[0].slug).toBe('official')
  })

  it('keeps site, owner and domain identities separate', () => {
    expect(domainIdentityLabel(domain, null)).toBe('访客')
    expect(domainIdentityLabel(domain, { ...member, role: 'admin' })).toBe('站点维护')
    expect(domainIdentityLabel(domain, { ...member, id: domain.ownerId! })).toBe('域 owner')
    expect(domainIdentityLabel(domain, member)).toBe('出题人')
    expect(domainIdentityLabel({ ...domain, memberRole: 'viewer' }, member)).toBe('只读成员')
  })

  it.each([
    ['', '未加入'],
    ['invited', '待接受邀请'],
    ['pending', '申请审核中'],
    ['suspended', '成员资格已停用'],
  ])('does not label an inactive member as an owner or active role (%s)', (status, expected) => {
    expect(
      domainIdentityLabel({ ...domain, memberStatus: status, ownerId: member.id }, member),
    ).toBe(expected)
  })
})
