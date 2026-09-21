import { sameValue, roleNames } from '@/lib/authoring-materials'

export const materialFieldNames: Record<string, string> = {
  rules: '生成规则',
  generator: '生成器',
  solution: '标准解',
  comparison: '答案比较规则',
  title: '题目名称',
  name: '名称',
  difficulty: '难度',
  timeLimitMs: '时间限制',
  memoryLimitKb: '内存限制',
  resourceMode: '限制方式',
  statementLanguage: '默认题面',
  source: '来源',
  license: '许可证',
  rightsOwner: '权利人',
  tags: '标签',
  mainSolution: '主参考解',
  inputValidators: '输入校验器',
  outputValidator: '输出校验器',
  testOrder: '测试顺序',
  requirements: '兼容要求',
  judgeType: '题目类型',
  role: '程序用途',
  language: '程序语言',
  protocol: '运行协议',
  directory: '工作目录',
  entryPoint: '主文件',
  files: '源文件',
  arguments: '运行参数',
  expectedVerdicts: '预期结果',
  isSample: '公开样例',
  isPretest: '预测试',
  group: '测试组',
  points: '分值',
  description: '说明',
  mode: '自测预期',
  input: '输入',
  answer: '答案',
  output: '候选输出',
  aggregation: '分组规则',
  maxScore: '分组分值',
  prerequisites: '依赖分组',
  'comparison.kind': '答案比较',
  'comparison.caseSensitive': '区分大小写',
  'comparison.spaceSensitive': '区分空白',
  'comparison.floatingPoint': '浮点比较',
  'comparison.absoluteTolerance': '绝对误差',
  'comparison.relativeTolerance': '相对误差',
  'input.kind': '输入来源',
  'input.entry': '输入文件',
  'input.generator': '生成器',
  'input.arguments': '生成参数',
  'answer.kind': '答案来源',
  'answer.entry': '答案文件',
  'answer.solution': '答案参考解',
}
function flatten(value: Record<string, unknown>, prefix = ''): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value).flatMap(([key, item]) => {
      const path = prefix ? `${prefix}.${key}` : key
      if (key === 'schemaVersion') return []
      return item && typeof item === 'object' && !Array.isArray(item)
        ? Object.entries(flatten(item as Record<string, unknown>, path))
        : [[path, item]]
    }),
  )
}
export function materialChanges(before: string, after: string) {
  try {
    const a = flatten(JSON.parse(before || '{}')),
      b = flatten(JSON.parse(after || '{}'))
    return [...new Set([...Object.keys(a), ...Object.keys(b)])]
      .filter(
        (key) =>
          !sameValue(a[key], b[key]) &&
          (before.trim() ||
            !(
              b[key] === false ||
              b[key] === 0 ||
              b[key] === '' ||
              (Array.isArray(b[key]) && !(b[key] as unknown[]).length)
            )),
      )
      .map((key) => ({ key, label: materialFieldNames[key] ?? key, before: a[key], after: b[key] }))
  } catch {
    return null
  }
}
export function materialValue(key: string, value: unknown, labels: Record<string, string>): string {
  if (value === undefined || value === null || value === '') return '未设置'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (Array.isArray(value)) {
    if (!value.length) return '无'
    if (key === 'testOrder')
      return (
        value
          .slice(0, 8)
          .map((id) => labels[String(id)] ?? '测试点')
          .join(' → ') + (value.length > 8 ? ` …（共 ${value.length} 个）` : '')
      )
    return value
      .map((item) =>
        typeof item === 'string'
          ? (labels[item] ??
            (
              {
                Accepted: '应通过',
                'Wrong Answer': '应判错',
                'Time Limit Exceeded': '应超时',
              } as Record<string, string>
            )[item] ??
            item)
          : typeof item === 'object' && item?.message
            ? String(item.message)
            : JSON.stringify(item),
      )
      .join('、')
  }
  if (key === 'timeLimitMs') return value === 0 ? '使用题目限制' : `${value} ms`
  if (key === 'memoryLimitKb') return value === 0 ? '使用题目限制' : `${Number(value) / 1024} MiB`
  if (typeof value === 'string') {
    if (labels[value]) return labels[value]
    const translated: Record<string, string> = {
      ...roleNames,
      file: '上传文件',
      tokens: '标准比较',
      exact: '精确',
      stdio: '标准输入输出',
      'language-scaled': '站点语言倍率',
      normal: '传统题',
      invalid_input: '应拒绝的输入',
      invalid_output: '应拒绝的输出',
      valid_output: '应接受的输出',
    }
    return translated[value] ?? value
  }
  return String(value)
}
