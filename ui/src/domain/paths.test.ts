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
    expect(domainPath('training', '/admin/contests')).toBe('/d/training/workspace/contests')
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

  it('keeps workbench and legacy links in the current domain, including login return paths', () => {
    expect(domainPath('training', '/workspace')).toBe('/d/training/workspace')
    expect(domainPath('training', '/authoring?visibility=draft')).toBe(
      '/d/training/workspace/problems?visibility=draft',
    )
    expect(domainPath('training', '/admin/problems')).toBe('/d/training/workspace/problems')
    expect(domainPath('training', '/manage/contests?page=2')).toBe(
      '/d/training/workspace/contests?page=2',
    )
    expect(domainReturnState('training', { from: '/workspace/contests' })).toEqual({
      from: '/d/training/workspace/contests',
    })
    expect(domainPath('training', '/domains?create=1')).toBe('/domains?create=1')
    expect(switchDomainPath('official', '/d/training/workspace/contests')).toBe(
      '/d/official/workspace/contests',
    )
    expect(switchDomainPath('official', '/d/training/authoring/1234')).toBe(
      '/d/official/workspace/problems',
    )
    expect(switchDomainPath('official', '/d/training/manage/contests')).toBe(
      '/d/official/workspace/contests',
    )
  })
})
