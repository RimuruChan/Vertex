import type {
  DomainMaterialView,
  DomainWorkingCopy,
  DtoProblemResponse,
} from '@/generated/api/model'
import { defaultProgram, defaultTest } from '@/lib/authoring-materials'
import type { createMockAPI } from './api'

// Deliberately opt-in: opening a real or demo problem never replaces its draft.
export function createAuthoringExample(
  api: ReturnType<typeof createMockAPI>,
  slug: string,
): string {
  const prefix = `/api/domains/${slug}`
  const problem = api.handle({
    method: 'POST',
    path: prefix + '/admin/problems',
    body: { title: '出题工作台 · A + B', visibility: 'private' },
  }) as DtoProblemResponse
  const path = prefix + '/authoring/problems/' + problem.id
  let copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
  function save(
    id: string,
    kind: string,
    pathName: string,
    text: string,
    attributes: Record<string, string> = {},
  ) {
    copy = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/' + id,
      body: { etag: copy.etag, entry: { id, kind, path: pathName, attributes }, text },
    }) as DomainWorkingCopy
  }
  save(
    'statement-zh',
    'statement',
    'statement/problem.zh.md',
    '# A + B\n\n读入两个整数，输出它们的和。\n\n## 输入\n\n一行两个整数，每个数的绝对值不超过 10⁹。\n\n## 输出\n\n输出它们的和。\n',
    { language: 'zh', format: 'markdown' },
  )
  save(
    'accepted-source',
    'source',
    'solutions/accepted/main.cpp',
    '#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<"\\n";}\n',
  )
  save(
    'wrong-source',
    'source',
    'solutions/wrong/main.cpp',
    '#include <iostream>\nint main(){std::cout<<0<<"\\n";}\n',
  )
  for (const [id, name, expected] of [
    ['accepted', '正确参考解', 'Accepted'],
    ['wrong', '错误参考解', 'Wrong Answer'],
  ]) {
    save(
      id,
      'program',
      `vertex/programs/${id}.json`,
      JSON.stringify({
        ...defaultProgram(),
        name,
        role: 'solution',
        directory: `solutions/${id}`,
        files: [`${id}-source`],
        entryPoint: `${id}-source`,
        expectedVerdicts: [expected],
      }),
      { format: 'json' },
    )
  }
  for (let index = 1; index <= 6; index++) {
    save(`input-${index}`, 'input', `data/${index}.in`, `${index} ${index + 1}\n`)
    save(`answer-${index}`, 'answer', `data/${index}.ans`, `${2 * index + 1}\n`)
    save(
      `case-${index}`,
      'test',
      `vertex/tests/case-${index}.json`,
      JSON.stringify({
        ...defaultTest(),
        name: index === 1 ? '基础样例' : `测试 ${index}`,
        isSample: index === 1,
        input: { kind: 'file', entry: `input-${index}`, generator: '', arguments: [] },
        answer: { kind: 'file', entry: `answer-${index}`, solution: '' },
      }),
      { format: 'json' },
    )
  }
  save('invalid-output', 'answer', 'validation/wrong.out', '0\n')
  for (const [id, name, mode, output] of [
    ['validation-accept', '正确输出', 'valid_output', 'answer-1'],
    ['validation-reject', '错误输出', 'invalid_output', 'invalid-output'],
  ])
    save(
      id,
      'validation',
      `vertex/validation/${id}.json`,
      JSON.stringify({
        schemaVersion: 1,
        name,
        mode,
        input: 'input-1',
        answer: 'answer-1',
        output,
        description: '验证默认输出比较器的正反例',
      }),
      { format: 'json' },
    )
  const material = api.handle({
    method: 'GET',
    path: path + '/materials/problem',
  }) as DomainMaterialView
  save(
    'problem',
    'metadata',
    'vertex/problem.json',
    JSON.stringify({ ...material.metadata, mainSolution: 'accepted' }),
    { format: 'json' },
  )
  api.handle({
    method: 'POST',
    path: path + '/commits',
    body: {
      etag: copy.etag,
      message: '准备题面、参考解与六组数据',
      requestId: crypto.randomUUID(),
    },
  })
  return problem.id
}
