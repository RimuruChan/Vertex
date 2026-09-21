import {
  exampleTitle,
  statementZH,
  statementEN,
  referenceSources,
  generatorCode,
  validatorCode,
} from './authoring-example-content'
import type {
  DomainGenerationResult,
  DomainCheckRun,
  DomainCommitOutcome,
} from '@/generated/api/model'
import type {
  DomainMaterialView,
  DomainWorkingCopy,
  DtoProblemResponse,
} from '@/generated/api/model'
import { defaultProgram, defaultTest, defaultGroup } from '@/lib/authoring-materials'
import type { createMockAPI } from './api'

// Deliberately opt-in: opening a real or demo problem never replaces its draft.
export function createAuthoringExample(
  api: ReturnType<typeof createMockAPI>,
  slug: string,
  scenario: 'complete' | 'grouped' = 'complete',
): string {
  const prefix = `/api/domains/${slug}`
  const problem = api.handle({
    method: 'POST',
    path: prefix + '/admin/problems',
    body: {
      title: exampleTitle + (scenario === 'grouped' ? ' · 分组草稿' : ''),
      visibility: 'private',
    },
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
  save('statement-zh', 'statement', 'statement/problem.zh.md', statementZH, {
    language: 'zh',
    format: 'markdown',
  })
  save('statement-en', 'statement', 'statement/problem.en.md', statementEN, {
    language: 'en',
    format: 'markdown',
  })
  const sourceIds: string[] = []
  for (const [name, content] of Object.entries(referenceSources)) {
    const id =
      name === 'main.cpp' ? 'accepted-source' : 'accepted-' + name.replace(/[^a-z0-9]/gi, '-')
    sourceIds.push(id)
    save(id, 'source', `solutions/accepted/${name}`, content, { label: name, language: 'cpp' })
  }
  save(
    'wrong-source',
    'source',
    'solutions/wrong/main.cpp',
    '#include <iostream>\n#include <vector>\nint main(){int n,q;std::cin>>n>>q;std::vector<int> p(n+1);for(int i=1;i<=n;i++){int x;std::cin>>x;p[i]=p[i-1]+x;}while(q--){int l,r;std::cin>>l>>r;std::cout<<p[r]-p[l-1]<<"\\n";}}\n',
    { language: 'cpp', label: 'main.cpp' },
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
        files: id === 'accepted' ? sourceIds : [`${id}-source`],
        entryPoint: `${id}-source`,
        expectedVerdicts: [expected],
      }),
      { format: 'json' },
    )
  }
  const cases: { name: string; values: number[]; queries: [number, number][] }[] = [
    {
      name: '基础样例',
      values: [1, 2, 3, 4, 5],
      queries: [
        [1, 3],
        [2, 5],
      ],
    },
    {
      name: '负数样例',
      values: [-3, 2, -5, 7],
      queries: [
        [1, 3],
        [2, 4],
        [3, 3],
      ],
    },
    { name: '单个元素', values: [1000000000], queries: [[1, 1]] },
    {
      name: '64 位整数边界',
      values: Array(8).fill(1000000000),
      queries: [
        [1, 8],
        [1, 4],
        [5, 8],
      ],
    },
    {
      name: '全零与重复查询',
      values: Array(10).fill(0),
      queries: [
        [1, 10],
        [3, 3],
        [1, 10],
      ],
    },
    {
      name: '正负抵消',
      values: [1000000000, -1000000000, 5, -5, 7, -7],
      queries: [
        [1, 6],
        [1, 3],
        [2, 5],
      ],
    },
  ]
  for (const [position, item] of cases.entries()) {
    const index = position + 1
    const input = `${item.values.length} ${item.queries.length}\n${item.values.join(' ')}\n${item.queries.map((q) => q.join(' ')).join('\n')}\n`
    const answer =
      item.queries
        .map(([l, r]) => item.values.slice(l - 1, r).reduce((a, b) => a + b, 0))
        .join('\n') + '\n'
    save(`input-${index}`, 'input', `data/${index}.in`, input)
    save(`answer-${index}`, 'answer', `data/${index}.ans`, answer)
    save(
      `case-${index}`,
      'test',
      `vertex/tests/case-${index}.json`,
      JSON.stringify({
        ...defaultTest(),
        name: item.name,
        isSample: index <= 2,
        input: { kind: 'file', entry: `input-${index}`, generator: '', arguments: [] },
        answer: { kind: 'file', entry: `answer-${index}`, solution: '' },
      }),
      { format: 'json', label: item.name },
    )
  }
  for (const [id, label, role, code] of [
    ['generator', '随机区间生成器', 'generator', generatorCode],
    ['validator', '输入范围校验器', 'input-validator', validatorCode],
  ]) {
    save(id + '-source', 'source', `programs/${id}/main.py`, code, {
      language: 'python',
      label: 'main.py',
    })
    const files = [id + '-source']
    if (id === 'generator') {
      save(
        'generator-patterns',
        'source',
        'programs/generator/patterns.py',
        'def make_values(n, bound, rng):\n    return [rng.randint(-bound, bound) for _ in range(n)]\n',
        { language: 'python', label: 'patterns.py' },
      )
      files.push('generator-patterns')
    }
    save(
      id,
      'program',
      `vertex/programs/${id}.json`,
      JSON.stringify({
        ...defaultProgram(),
        name: label,
        role,
        language: 'python',
        directory: `programs/${id}`,
        files,
        entryPoint: id + '-source',
        expectedVerdicts: [],
        protocol: 'stdio',
      }),
      { format: 'json', label },
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
    JSON.stringify({
      ...material.metadata,
      mainSolution: 'accepted',
      inputValidators: ['validator'],
      tags: ['前缀和', '多次询问'],
      source: '出题流程演示',
    }),
    { format: 'json' },
  )
  const first = api.handle({
    method: 'POST',
    path: path + '/commits',
    body: {
      etag: copy.etag,
      message: '准备中英文题面、多文件参考解、校验器与六组边界数据',
      requestId: crypto.randomUUID(),
    },
  }) as DomainCommitOutcome
  copy = first.copy
  copy = (
    api.handle({
      method: 'POST',
      path: path + '/generation',
      body: {
        etag: copy.etag,
        id: 'coverage-plan',
        preview: false,
        plan: {
          schemaVersion: 1,
          name: '随机与规模覆盖',
          generator: 'generator',
          solution: 'accepted',
          group: '',
          rules: [
            {
              id: 'small',
              name: '小规模随机',
              count: 8,
              seedStart: 1,
              parameters: '20 30 {seed} 100',
            },
            {
              id: 'medium',
              name: '中规模随机',
              count: 6,
              seedStart: 100,
              parameters: '1000 1000 {seed} 1000000000',
            },
            {
              id: 'large',
              name: '最大规模',
              count: 4,
              seedStart: 1000,
              parameters: '200000 200000 {seed} 1000000000',
            },
          ],
        },
      },
    }) as DomainGenerationResult
  ).copy!
  const second = api.handle({
    method: 'POST',
    path: path + '/commits',
    body: {
      etag: copy.etag,
      message: '增加可复现的生成方案，覆盖随机和最大规模',
      requestId: crypto.randomUUID(),
    },
  }) as DomainCommitOutcome
  copy = second.copy
  for (const revision of [1, 2]) {
    api.handle({ method: 'POST', path: path + '/checks', body: { revision } }) as DomainCheckRun
    for (let step = 0; step < 3; step++) api.handle({ method: 'GET', path: path + '/checks' })
  }
  save(
    'statement-zh',
    'statement',
    'statement/problem.zh.md',
    statementZH + '\n提示：单次询问的答案可能超过 32 位整数范围。\n',
    { language: 'zh', format: 'markdown' },
  )
  save(
    'author-notes',
    'resource',
    'notes/coverage.md',
    '# 数据设计记录\n\n- 小规模覆盖区间端点。\n- 最大规模验证线性预处理。\n- 大权值拦截 32 位溢出错误解。\n',
    { label: '数据设计记录.md' },
  )
  if (scenario === 'grouped') {
    save(
      'draft-subtask',
      'group',
      'vertex/groups/draft-subtask.json',
      JSON.stringify({
        ...defaultGroup(),
        name: '基础子任务（草稿）',
        description: '演示分组配置及当前提交评测的能力限制。',
      }),
      { format: 'json', label: '基础子任务（草稿）' },
    )
    const test = api.handle({
      method: 'GET',
      path: path + '/materials/case-3',
    }) as DomainMaterialView
    save(
      'case-3',
      'test',
      'vertex/tests/case-3.json',
      JSON.stringify({ ...test.test!, group: 'draft-subtask' }),
      { format: 'json', label: test.test!.name },
    )
  }
  return problem.id
}
