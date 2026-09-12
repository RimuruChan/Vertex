import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { MemoryRouter } from 'react-router-dom'
import JudgeResultPanel from './JudgeResultPanel'
import { createFixtures } from '@/mocks/fixtures'

function render(feedback: string, status = 'Wrong Answer') {
  const submission = {
    ...createFixtures().submissions[0],
    status,
    totalTimeMs: 4321,
    compileResult: 'PRIVATE LOG',
  }
  return renderToStaticMarkup(
    <MemoryRouter>
      <JudgeResultPanel submission={submission} feedback={feedback} />
    </MemoryRouter>,
  )
}

describe('compact submission feedback', () => {
  it('explains withheld results without revealing stale verdicts or case data', () => {
    const html = render('none')
    expect(html).toContain('提交已收到，赛后公布结果')
    expect(html).not.toContain('答案错误')
    expect(html).not.toContain('测试点结果分布')
    expect(html).not.toContain('4321')
    expect(html).not.toContain('PRIVATE LOG')
  })
  it('shows summary verdicts but explains why detailed data is unavailable', () => {
    const html = render('summary')
    expect(html).toContain('答案错误')
    expect(html).toContain('本场仅公布最终判定')
    expect(html).not.toContain('测试点结果分布')
    expect(html).not.toContain('4321')
    expect(html).not.toContain('PRIVATE LOG')
  })
  it('keeps full-feedback details and the new-tab link available', () => {
    const html = render('full')
    expect(html).toContain('测试点结果分布')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('4321')
  })
})
