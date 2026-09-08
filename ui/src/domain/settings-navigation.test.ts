import { describe, expect, it } from 'vitest'
import { visibleDomainSettings } from './settings-navigation'

const member = { permissions: [], canArchive: false, canTransfer: false }

describe('domain settings navigation', () => {
  it('does not expose management navigation to ordinary members', () => {
    expect(visibleDomainSettings(member)).toEqual([])
  })

  it('opens the section that a delegated manager can actually manage', () => {
    expect(
      visibleDomainSettings({ ...member, permissions: ['domain.members.manage'] }).map(
        (item) => item.path,
      ),
    ).toEqual(['/settings/members'])
    expect(visibleDomainSettings({ ...member, permissions: ['domain.groups.manage'] })).toEqual([])
  })

  it('keeps domain recovery reachable when archiving removes ordinary permissions', () => {
    expect(visibleDomainSettings({ ...member, canArchive: true }).map((item) => item.path)).toEqual(
      ['/settings'],
    )
    expect(
      visibleDomainSettings({ ...member, canTransfer: true }).map((item) => item.path),
    ).toEqual(['/settings'])
  })
})
