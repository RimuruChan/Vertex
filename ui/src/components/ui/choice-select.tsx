import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './select'

export function ChoiceSelect({
  id,
  value,
  onValueChange,
  options,
  disabled,
  label,
}: {
  id?: string
  value: string
  onValueChange: (value: string) => void
  options: ReadonlyArray<readonly [string, string]>
  disabled?: boolean
  label?: string
}) {
  return (
    <Select value={value} onValueChange={onValueChange} disabled={disabled}>
      <SelectTrigger id={id} aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map(([key, text]) => (
          <SelectItem key={key} value={key}>
            {text}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
