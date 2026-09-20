import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
export function Choice({
  label,
  value,
  onChange,
  options,
  disabled = false,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: [string, string][]
  disabled?: boolean
}) {
  return (
    <Field label={label}>
      <Select
        value={value || '__none'}
        onValueChange={(value) => onChange(value === '__none' ? '' : value)}
        disabled={disabled}
      >
        <SelectTrigger aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(([id, name]) => (
            <SelectItem key={id || '__none'} value={id || '__none'}>
              {name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}
export function Field({
  label,
  children,
  hint,
}: {
  label: string
  children: ReactNode
  hint?: string
}) {
  return (
    <div className="min-w-0 space-y-1.5">
      <Label>{label}</Label>
      {children}
      {hint && <p className="text-xs leading-5 text-muted-foreground">{hint}</p>}
    </div>
  )
}

export function NumberField({
  label,
  unit,
  value,
  onChange,
  disabled,
  min = 1,
  max = Infinity,
  integer = false,
}: {
  label: string
  unit: string
  value: number
  onChange: (value: number) => void
  disabled: boolean
  min?: number
  max?: number
  integer?: boolean
}) {
  const [draft, setDraft] = useState(String(value)),
    focused = useRef(false)
  useEffect(() => {
    if (!focused.current) setDraft(String(value))
  }, [value])
  const invalid =
    draft === '' ||
    !Number.isFinite(Number(draft)) ||
    Number(draft) < min ||
    Number(draft) > max ||
    (integer && !Number.isInteger(Number(draft)))
  return (
    <Field label={label}>
      <div className="relative">
        <Input
          aria-label={label}
          type="number"
          min={min}
          step={integer ? 1 : 'any'}
          value={draft}
          disabled={disabled}
          aria-invalid={invalid}
          className="pr-14"
          onFocus={() => {
            focused.current = true
          }}
          onBlur={() => {
            focused.current = false
          }}
          onChange={(e) => {
            setDraft(e.target.value)
            onChange(Number(e.target.value))
          }}
        />
        <span className="pointer-events-none absolute right-3 top-3 text-xs text-muted-foreground">
          {unit}
        </span>
      </div>
      {invalid && (
        <p role="alert" className="mt-1 text-xs text-destructive">
          请填写有效的{label}
        </p>
      )}
    </Field>
  )
}
export function TagsField({
  value,
  onChange,
  disabled,
}: {
  value: string[]
  onChange: (value: string[]) => void
  disabled: boolean
}) {
  const [draft, setDraft] = useState(value.join(', ')),
    focused = useRef(false)
  useEffect(() => {
    if (!focused.current) setDraft(value.join(', '))
  }, [value.join('|')])
  return (
    <Field label="标签" hint="用逗号分隔多个标签。">
      <Input
        aria-label="标签"
        value={draft}
        disabled={disabled}
        placeholder="例如：图论, 最短路"
        onFocus={() => {
          focused.current = true
        }}
        onBlur={() => {
          focused.current = false
          setDraft(value.join(', '))
        }}
        onChange={(event) => {
          setDraft(event.target.value)
          onChange([
            ...new Set(
              event.target.value
                .split(/[,，]/)
                .map((item) => item.trim())
                .filter(Boolean),
            ),
          ])
        }}
      />
    </Field>
  )
}
