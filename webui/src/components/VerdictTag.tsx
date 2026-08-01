import { Tag } from 'antd'

// 判定 → 颜色与图标映射(与后端状态一致)。
const verdictColors: Record<string, { color: string; label: string }> = {
  Accepted: { color: 'green', label: 'AC' },
  'Wrong Answer': { color: 'red', label: 'WA' },
  'Time Limit Exceeded': { color: 'orange', label: 'TLE' },
  'Memory Limit Exceeded': { color: 'orange', label: 'MLE' },
  'Runtime Error': { color: 'volcano', label: 'RE' },
  'Compile Error': { color: 'purple', label: 'CE' },
  'Output Limit Exceeded': { color: 'gold', label: 'OLE' },
  'System Error': { color: 'magenta', label: 'SE' },
  Pending: { color: 'default', label: '排队中' },
  Judging: { color: 'processing', label: '评测中' },
  Skipped: { color: 'default', label: '跳过' },
}

// VerdictTag:判定状态的彩色标签(用于提交列表/详情)。
export default function VerdictTag({ status }: { status: string }) {
  const v = verdictColors[status] ?? { color: 'default', label: status }
  return <Tag color={v.color}>{v.label}</Tag>
}
