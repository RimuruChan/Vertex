import type {
  DtoContestResponse,
  DtoClarificationResponse,
  DtoDiscussionResponse,
  DtoEditorialResponse,
  DtoProblemResponse,
  DtoSetResponse,
  DtoSubmissionResponse,
  DtoUserResponse,
} from '@/generated/api/model'

export const demoUser: DtoUserResponse = {
  id: '00000001-0000-4000-8000-000000000001',
  username: 'demo',
  email: 'demo@example.test',
  role: 'user',
}

export const mockID = (kind: number, index: number) =>
  `${String(kind).padStart(4, '0')}${String(index).padStart(4, '0')}-0000-4000-8000-000000000001`

const problemSpecs: [string, number, string[], string, string, string, string, string][] = [
  [
    '两数之和',
    2,
    ['数组', '哈希表'],
    '给定一个整数数组和目标值，找到两个不同位置的数，使它们的和等于目标值。输出这两个位置的下标（从 1 开始）。保证答案唯一。',
    '第一行是 n 和 target，第二行是 n 个整数。n 不超过 200000。',
    '两个下标，按升序输出。',
    '4 9\n2 7 11 15',
    '1 2',
  ],
  [
    'A + B',
    1,
    ['入门', '数学'],
    '读入两个整数 a 和 b，计算它们的和。',
    '一行两个整数，绝对值不超过 10 的 9 次方。',
    '输出 a + b。',
    '3 5',
    '8',
  ],
  [
    '最长不重复子串',
    4,
    ['字符串', '双指针'],
    '给定一个小写字母字符串，求不含重复字符的最长连续子串的长度。',
    '一行字符串 s，长度不超过 200000。',
    '最长子串的长度。',
    'abcabcbb',
    '3',
  ],
  [
    '区间求和',
    3,
    ['前缀和', '数组'],
    '给定一个整数序列，回答若干次闭区间和查询。下标从 1 开始。',
    '第一行 n 和 q；第二行 n 个整数；接下来 q 行，每行 l 和 r。n、q 均不超过 200000。',
    '每行输出一次查询的结果。',
    '5 2\n1 2 3 4 5\n1 3\n2 5',
    '6\n14',
  ],
  [
    '合并区间',
    4,
    ['排序', '贪心'],
    '合并所有相交的闭区间，输出按左端点递增排列的结果。端点相同也视为相交。',
    '第一行 n，接下来 n 行各包含两个端点。n 不超过 200000。',
    '每行一个合并后的区间。',
    '3\n1 3\n2 6\n8 10',
    '1 6\n8 10',
  ],
  [
    '最少硬币',
    5,
    ['动态规划'],
    '有 n 种面值的硬币，每种数量不限。凑出金额 m，最少需要多少枚？',
    '第一行 n 和 m，第二行 n 种正整数面值。n 不超过 100，m 不超过 10000。',
    '最少枚数；无解输出 -1。',
    '3 11\n1 2 5',
    '3',
  ],
  [
    '网格寻路',
    4,
    ['搜索', '图论'],
    '从网格左上角走到右下角，每次上下左右移动一格。点号可通过，井号不可通过，求最少步数。',
    '第一行 n 和 m，随后 n 行网格。n、m 不超过 1000。',
    '最少步数；不可达输出 -1。',
    '3 3\n...\n.#.\n...',
    '4',
  ],
  [
    '括号序列',
    2,
    ['栈', '字符串'],
    '判断一个仅由圆括号组成的字符串是否为合法括号序列。',
    '一行长度不超过 200000 的字符串。',
    '合法输出 Yes，否则输出 No。',
    '(())()',
    'Yes',
  ],
  [
    '最长递增子序列',
    6,
    ['动态规划', '二分'],
    '求整数序列的最长严格递增子序列长度，子序列不要求连续。',
    '第一行 n，第二行 n 个整数。n 不超过 200000。',
    '输出最长长度。',
    '6\n3 1 4 1 5 9',
    '4',
  ],
  [
    '最短路径',
    6,
    ['图论', '最短路'],
    '给定正权有向图，求从顶点 1 到顶点 n 的最短距离。',
    '第一行 n 和 m，接下来 m 行 u、v、w。n 不超过 100000，m 不超过 200000。',
    '最短距离；不可达输出 -1。',
    '3 3\n1 2 2\n2 3 3\n1 3 9',
    '5',
  ],
  [
    '连通分量',
    4,
    ['并查集', '图论'],
    '给定无向图，求图中连通分量的数量。孤立点也算一个连通分量。',
    '第一行 n 和 m，接下来 m 行每行两个端点。n、m 不超过 200000。',
    '连通分量数量。',
    '5 2\n1 2\n3 4',
    '3',
  ],
  [
    '区间最大值',
    7,
    ['数据结构', '线段树'],
    '维护一个整数数组，支持单点赋值和区间最大值查询。',
    '第一行 n 和 q，第二行初始数组；接下来 q 行操作：1 i x 表示赋值，2 l r 表示查询。n、q 不超过 200000。',
    '每次查询的最大值。',
    '3 3\n1 4 2\n2 1 3\n1 2 0\n2 1 3',
    '4\n2',
  ],
]

export function createFixtures(now = Date.now()) {
  const ago = (hours: number) => new Date(now - hours * 3600000).toISOString()
  const problems: DtoProblemResponse[] = problemSpecs.map(
    ([title, difficulty, tags, statement, input, output, sampleIn, sampleOut], i) => ({
      id: mockID(1000, i + 1),
      title,
      difficulty,
      tags,
      statementMd: `${statement}\n\n## 输入格式\n\n${input}\n\n## 输出格式\n\n${output}\n\n## 样例输入\n\n\`\`\`text\n${sampleIn}\n\`\`\`\n\n## 样例输出\n\n\`\`\`text\n${sampleOut}\n\`\`\``,
      timeLimitMs: 1000,
      memoryLimitKb: 262144,
      judgeType: 'standard',
      source: i < 4 ? '基础算法练习' : 'Vertex 训练集',
      visibility: 'public',
      createdAt: ago(720),
      updatedAt: ago(48),
      submissionCount: 1200 - i * 73,
      acceptedCount: 820 - i * 57,
      solvedUserCount: 480 - i * 31,
      userStatus: 'none',
    }),
  )
  const submissions: DtoSubmissionResponse[] = Array.from({ length: 26 }, (_, i) => {
    const problem = problems[i % problems.length]
    const status =
      i === 0 || i % 7 === 0 ? 'Wrong Answer' : i % 5 === 0 ? 'Time Limit Exceeded' : 'Accepted'
    return {
      id: mockID(2000, i + 1),
      problemId: problem.id,
      problemTitle: problem.title,
      userId: i % 3 === 2 ? '00000002-0000-4000-8000-000000000001' : demoUser.id,
      username: i % 3 === 2 ? 'lin' : demoUser.username,
      language: i % 4 === 0 ? 'python' : 'cpp',
      status,
      score: status === 'Accepted' ? 100 : 40,
      submittedAt: ago((i + 1) * 0.65),
      judgedAt: ago((i + 1) * 0.65 - 0.001),
      totalTimeMs: status === 'Time Limit Exceeded' ? 1000 : 12 + i * 7,
      peakMemoryKb: 3200 + i * 128,
      judgedCases: 5,
      totalCases: 5,
      sourceCode:
        i % 4 === 0
          ? '# 示例代码，仅用于界面演示\nprint(sum(map(int, input().split())))\n'
          : '// 示例代码，仅用于界面演示\n#include <iostream>\nint main() {\n    long long a, b;\n    std::cin >> a >> b;\n    std::cout << a + b << "\\n";\n}\n',
      caseResults: Array.from({ length: 5 }, (_, c) => ({
        caseIndex: c + 1,
        verdict: c < 2 ? 'Accepted' : status,
        timeMs: 3 + c,
        memoryKb: 3200,
      })),
    }
  })
  const contests: DtoContestResponse[] = [
    {
      title: '周末练习赛 · 03',
      beginAt: ago(0.5),
      endAt: ago(-2),
      description:
        '留出两小时，专注解决几个有意思的问题。包含基础算法、搜索与动态规划，适合独立练习。',
    },
    {
      title: '算法入门挑战',
      beginAt: ago(-26),
      endAt: ago(-28),
      description: '从第一份 Accepted 开始。六道基础题目，熟悉比赛节奏。',
    },
    {
      title: '夏日算法练习赛',
      beginAt: ago(192),
      endAt: ago(189),
      description: '比赛已经结束，题目开放练习，欢迎交流不同的解法。',
    },
  ].map((contest, i) => ({
    ...contest,
    id: mockID(3000, i + 1),
    createdAt: ago(360),
    visibility: 'public',
    rule: 'icpc',
    format: 'icpc',
    feedback: 'full',
    penaltyMinutes: 20,
    penalizeCompileError: false,
    rankboardVisible: true,
  }))
  const titles = [
    '用哈希表记住已经走过的路',
    '从输入输出开始',
    '滑动窗口：让每个字符只经过两次',
    '前缀和为什么有效',
    '先排序，再合并',
    '把大问题拆成小问题',
  ]
  const editorials: DtoEditorialResponse[] = titles.map((title, i) => ({
    id: mockID(4000, i + 1),
    problemId: problems[i].id,
    problemTitle: problems[i].title,
    title,
    authorId: i === 1 ? demoUser.id : '00000002-0000-4000-8000-000000000001',
    authorName: i === 1 ? 'demo' : 'lin',
    contentMd:
      i === 0
        ? '## 从暴力解法出发\n\n枚举两个数很直接，但需要 $O(n^2)$ 的时间。我们真正需要知道的是：**当前数的另一半，是否已经出现过？**\n\n## 用空间换时间\n\n遍历数组时，用哈希表记录每个数及其下标。对于当前的 $x$，查询 $target - x$。先查找，再插入，就不会重复使用同一个位置。\n\n```cpp\nfor (int i = 0; i < n; ++i) {\n    auto it = seen.find(target - a[i]);\n    if (it != seen.end()) {\n        cout << it->second + 1 << " " << i + 1;\n        return 0;\n    }\n    seen[a[i]] = i;\n}\n```\n\n## 复杂度\n\n期望时间 $O(n)$，空间 $O(n)$。\n\n可以继续思考：如果要求输出所有数对，需要怎样调整？'
        : `## 思路\n\n${problems[i].title}的关键在于找到可以复用的信息。先从样例出发，手动走一遍，再选择合适的数据结构。\n\n## 检查边界\n\n- 最小规模是否成立？\n- 是否可能出现重复元素？\n- 整数范围是否需要使用 64 位类型？\n\n欢迎在下方讨论中分享自己的思路。`,
    createdAt: ago(6 + i * 24),
    updatedAt: ago(6 + i * 24),
    voteCount: 48 - i * 6,
    voted: false,
    canEdit: i === 1,
    locked: false,
    status: 'published',
    visibility: 'public',
    solvedOnly: i === 5,
  }))
  const discussions: DtoDiscussionResponse[] = [
    {
      id: 1,
      problemId: problems[0].id,
      contentMd: '如果数组里有重复元素，比如 `[3, 3]`，是不是要先查询再存入哈希表？',
      authorName: 'lin',
      authorId: '00000002-0000-4000-8000-000000000001',
      createdAt: ago(3),
      updatedAt: ago(3),
      edited: false,
    },
    {
      id: 2,
      problemId: problems[0].id,
      parentId: 1,
      contentMd: '是的，先查找能保证不会使用同一个下标两次。',
      authorName: 'demo',
      authorId: demoUser.id,
      createdAt: ago(2),
      updatedAt: ago(2),
      edited: false,
    },
    {
      id: 3,
      editorialId: editorials[0].id,
      contentMd: '「先查询，再插入」这个细节讲得很清楚。排序 + 双指针也是一种思路。',
      authorName: 'demo',
      authorId: demoUser.id,
      createdAt: ago(1),
      updatedAt: ago(1),
      edited: false,
    },
  ]
  const sets: DtoSetResponse[] = ['从零开始的算法之旅', '数组与字符串', '图论第一课'].map(
    (title, i) => ({
      id: mockID(5000, i + 1),
      title,
      description: [
        '循序渐进，从输入输出到独立解决问题。每次完成一点，慢慢建立自己的算法工具箱。',
        '把常见的数组与字符串技巧串起来，理解哈希表、双指针和前缀和。',
        '从网格搜索开始，走向连通性与最短路径。',
      ][i],
      authorId: demoUser.id,
      authorName: 'demo',
      canEdit: true,
      visibility: 'public',
      createdAt: ago(120),
      updatedAt: ago(24),
      problemCount: 0,
      solvedCount: 0,
      items: (i === 0
        ? problems.slice(0, 8)
        : i === 1
          ? problems.slice(0, 5)
          : [problems[6], problems[9], problems[10]]
      ).map((p, index) => ({
        problemId: p.id,
        title: p.title,
        difficulty: p.difficulty,
        tags: p.tags,
        note: index === 0 ? '从这道题开始' : '',
        sortOrder: index,
        visibility: p.visibility,
        userStatus: p.userStatus,
        acceptCount: p.acceptedCount,
        submitCount: p.submissionCount,
      })),
    }),
  )
  return {
    user: { ...demoUser } as DtoUserResponse | null,
    problems,
    submissions,
    contests,
    editorials,
    discussions,
    sets,
    registrations: [] as string[],
    clarifications: {} as Record<string, DtoClarificationResponse[]>,
    pending: {} as Record<string, { started: number; verdict: string }>,
  }
}

export type MockState = ReturnType<typeof createFixtures>
