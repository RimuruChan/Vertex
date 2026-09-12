import { describe, expect, it } from 'vitest'
import { validationMessage } from './validation-message'

describe('validationMessage', () => {
  const field = {
    validity: {} as ValidityState,
    type: 'text',
    minLength: 6,
    maxLength: 72,
    min: '1',
    max: '100',
    validationMessage: '',
  }
  it.each([
    ['valueMissing', '请填写此项'],
    ['tooShort', '至少需要 6 个字符'],
    ['tooLong', '最多允许 72 个字符'],
    ['rangeUnderflow', '不能小于 1'],
    ['rangeOverflow', '不能大于 100'],
    ['badInput', '请输入有效的数字'],
    ['stepMismatch', '请输入符合步长要求的数值'],
    ['patternMismatch', '请按要求的格式填写'],
  ])('explains %s without including entered values', (flag, message) => {
    expect(
      validationMessage({ ...field, validity: { [flag]: true } as unknown as ValidityState }),
    ).toBe(message)
  })
  it('explains malformed email addresses', () => {
    expect(
      validationMessage({
        ...field,
        type: 'email',
        validity: { typeMismatch: true } as ValidityState,
      }),
    ).toBe('请输入有效的邮箱地址')
  })
  it('preserves custom validation errors', () => {
    expect(validationMessage({ ...field, validationMessage: '开始时间必须早于结束时间' })).toBe(
      '开始时间必须早于结束时间',
    )
  })
})
