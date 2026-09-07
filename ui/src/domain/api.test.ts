import { describe, expect, it, vi } from 'vitest'
vi.mock('@/generated/api/vertex', () => ({
  getApiDomainsDomainProblems: vi.fn(async (...args) => args),
  getApiDomainsDomainProblemsId: vi.fn(async (...args) => args),
  getApiProblems: vi.fn(async () => 'legacy'),
}))
import { bindDomainAPI } from './api'

describe('domain-bound generated clients', () => {
  it('captures the domain for every request without retargeting an earlier client', async () => {
    const official = bindDomainAPI('official'),
      training = bindDomainAPI('training')
    expect(await official.getApiProblems({ page: 1 })).toEqual(['official', { page: 1 }])
    expect(await training.getApiProblemsId('1000')).toEqual(['training', '1000'])
    expect(await official.getApiProblemsId('1000')).toEqual(['official', '1000'])
    expect(() => bindDomainAPI('../other')).toThrow()
  })
})
