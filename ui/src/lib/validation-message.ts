type Constraint = {
  validity: ValidityState
  type: string
  minLength: number
  maxLength: number
  min: string
  max: string
  validationMessage: string
}

export function validationMessage(field: Constraint): string {
  const v = field.validity
  if (v.valueMissing) return '请填写此项'
  if (v.tooShort) return `至少需要 ${field.minLength} 个字符`
  if (v.tooLong) return `最多允许 ${field.maxLength} 个字符`
  if (v.typeMismatch) return field.type === 'email' ? '请输入有效的邮箱地址' : '请输入有效的网址'
  if (v.rangeUnderflow) return `不能小于 ${field.min}`
  if (v.rangeOverflow) return `不能大于 ${field.max}`
  if (v.badInput) return '请输入有效的数字'
  if (v.stepMismatch) return '请输入符合步长要求的数值'
  if (v.patternMismatch) return '请按要求的格式填写'
  return field.validationMessage || '请检查此项内容'
}
