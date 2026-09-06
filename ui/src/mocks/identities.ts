import type { DtoUserResponse } from '@/generated/api/model'

function account(index: number, username: string, role = 'user'): DtoUserResponse {
  return {
    id: `0000000${index}-0000-4000-8000-000000000001`,
    username,
    email: `${username}@example.test`,
    role,
  }
}

export const demoUser = account(1, 'demo')
export const contestantUser = account(3, 'contestant')
export const juryUser = account(4, 'jury')
export const observerUser = account(5, 'observer')
export const adminUser = account(6, 'admin_demo', 'admin')
export const mockIdentities = [
  { label: '访客（未登录）', user: null },
  { label: '普通用户 · demo', user: demoUser },
  { label: '选手 · contestant', user: contestantUser },
  { label: '裁判 · jury', user: juryUser },
  { label: '观察员 · observer', user: observerUser },
  { label: '管理员 · admin_demo', user: adminUser },
]
export const mockUsers = [
  demoUser,
  account(2, 'lin'),
  contestantUser,
  juryUser,
  observerUser,
  adminUser,
]
