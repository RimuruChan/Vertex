import type {
  DtoBuildResponse,
  DtoFileResponse,
  DtoPackageMetaResponse,
  DtoStatementResponse,
  DtoTestResponse,
  DtoWorkspaceResponse,
} from '@/generated/api/model'

export type Workspace = DtoWorkspaceResponse
export type PackageMeta = DtoPackageMetaResponse
export type PackageFile = DtoFileResponse
export type PackageTest = DtoTestResponse
export type PackageStatement = DtoStatementResponse
export type PackageBuild = DtoBuildResponse

/** Package roles, ordered the way an author works through them. */
export const FILE_KINDS = ['solution', 'checker', 'validator', 'generator', 'interactor'] as const
export type FileKind = (typeof FILE_KINDS)[number]

export const KIND_LABELS: Record<string, string> = {
  solution: '解',
  checker: 'Checker',
  validator: 'Validator',
  generator: 'Generator',
  interactor: 'Interactor',
}

export const KIND_HINTS: Record<string, string> = {
  solution: '标程用于产生每个测试点的答案;其它解按预期判定参与构建期对拍。',
  checker: '特殊判定程序,用 testlib 编写。缺省使用忽略行尾空白的逐字节比较。',
  validator: '输入校验器,构建时对每个测试点运行一次,不通过则整次构建失败。',
  generator: '数据生成器,由测试点的生成命令按名字调用,参数原样传入 argv。',
  interactor: '交互题的交互器(交互题判题尚未开放)。',
}

/** testlib links against C++ only; generators and solutions may use any judged language. */
export const KIND_LANGUAGES: Record<string, string[]> = {
  solution: ['cpp', 'c', 'python'],
  checker: ['cpp'],
  validator: ['cpp'],
  interactor: ['cpp'],
  generator: ['cpp', 'python'],
}

export const LANGUAGE_LABELS: Record<string, string> = {
  cpp: 'C++',
  c: 'C',
  python: 'Python',
}

/** Verdicts an author may declare for an alternate solution. */
export const EXPECTED_VERDICTS = [
  { value: 'Accepted', label: 'Accepted' },
  { value: 'Wrong Answer', label: 'Wrong Answer' },
  { value: 'Time Limit Exceeded', label: 'Time Limit Exceeded' },
  { value: 'Memory Limit Exceeded', label: 'Memory Limit Exceeded' },
  { value: 'Runtime Error', label: 'Runtime Error' },
  { value: 'Any Rejection', label: '任意非 AC' },
]

export const BUILD_STAGE_LABELS: Record<string, string> = {
  queued: '排队中',
  compile: '编译',
  generate: '生成数据',
  validate: '校验输入',
  answer: '运行标程',
  check: '自检',
  solutions: '对拍',
  package: '打包',
  done: '完成',
}

export const BUILD_STATE_LABELS: Record<string, string> = {
  queued: '排队中',
  running: '构建中',
  succeeded: '成功',
  failed: '失败',
  cancelled: '已取消',
  dead: '已放弃',
}

export function isBuildActive(build: PackageBuild | undefined): boolean {
  return build?.state === 'queued' || build?.state === 'running'
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
