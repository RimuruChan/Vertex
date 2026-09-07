import { describe, expect, it } from 'vitest'
import { domainPath, domainReturnState, relativeDomainPath, switchDomainPath } from './paths'

describe('domain navigation', () => {
  it('qualifies resources and keeps global and already-qualified addresses intact', () => {
    expect(domainPath('training', '/problems/1000?tab=statement')).toBe(
      '/d/training/problems/1000?tab=statement',
    )
    expect(domainPath('training', '/login')).toBe('/login')
    expect(domainPath('training', '/announcements/2')).toBe('/d/training/announcements/2')
    expect(switchDomainPath('official', '/d/training/announcements/2')).toBe(
      '/d/official/announcements',
    )
    expect(domainPath('training', '/admin')).toBe('/admin')
    expect(domainPath('training', '/d/official/problems/1000')).toBe('/d/official/problems/1000')
    expect(domainPath('training', '/')).toBe('/d/training')
    expect(() => domainPath('../other', '/problems')).toThrow()
  })
  it('canonicalizes management links and drops resource identities on domain switches', () => {
    expect(domainPath('training', '/admin/problems/1000/package')).toBe(
      '/d/training/authoring/1000',
    )
    expect(domainPath('training', '/admin/contests')).toBe('/d/training/manage/contests')
    expect(switchDomainPath('official', '/d/training/contests/1/problems/A')).toBe(
      '/d/official/contests',
    )
    expect(switchDomainPath('official', '/d/training/users/demo')).toBe('/d/official')
    expect(relativeDomainPath('/d/training')).toBe('/')
    expect(domainReturnState('training', { from: '/problems/1000' })).toEqual({
      from: '/d/training/problems/1000',
    })
    expect(domainReturnState('official', { from: '/d/training/problems/1000' })).toEqual({
      from: '/d/training/problems/1000',
    })
  })
})
