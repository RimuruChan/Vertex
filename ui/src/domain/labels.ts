const roles: Record<string, string> = {
  admin: '域管理员',
  author: '出题人',
  member: '成员',
  viewer: '只读成员',
}
export const domainRoleLabel = (key: string) => roles[key] || key || '非成员'
