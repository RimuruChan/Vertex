import { useId, useLayoutEffect, useRef, useState } from 'react'
import { CalendarDays, ChevronLeft, ChevronRight, Clock3 } from 'lucide-react'
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from './dialog'
import { Button } from './button'
import { Input } from './input'
import { ChoiceSelect } from './choice-select'
import { cn } from '@/lib/utils'
import { calendarDays, localDateText, parseLocalDateTime } from '@/lib/date-time'

const hours = Array.from(
  { length: 24 },
  (_, i) => [String(i).padStart(2, '0'), String(i).padStart(2, '0')] as const,
)
const minutes = Array.from(
  { length: 60 },
  (_, i) => [String(i).padStart(2, '0'), String(i).padStart(2, '0')] as const,
)

export function DateTimePicker({
  id,
  value,
  onChange,
  disabled = false,
  required = false,
  min,
  max,
  className,
}: {
  id?: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  required?: boolean
  min?: string
  max?: string
  className?: string
}) {
  const prefix = useId()
  const [open, setOpen] = useState(false)
  const [date, setDate] = useState('')
  const [hour, setHour] = useState('09')
  const [minute, setMinute] = useState('00')
  const [cursor, setCursor] = useState(new Date())
  const focusDay = useRef(false)
  const cells = useRef(new Map<string, HTMLButtonElement>())
  const candidate = `${date}T${hour}:${minute}`
  const parsed = parseLocalDateTime(candidate)
  const beforeMin = parsed && min && parsed < (parseLocalDateTime(min) ?? parsed)
  const afterMax = parsed && max && parsed > (parseLocalDateTime(max) ?? parsed)
  const valid = !!parsed && !beforeMin && !afterMax
  function choose(date: Date) {
    setDate(localDateText(date))
    setHour(String(date.getHours()).padStart(2, '0'))
    setMinute(String(date.getMinutes()).padStart(2, '0'))
    setCursor(date)
  }
  useLayoutEffect(() => {
    if (focusDay.current) {
      cells.current.get(localDateText(cursor))?.focus()
      focusDay.current = false
    }
  }, [cursor])
  useLayoutEffect(() => {
    if (disabled) setOpen(false)
  }, [disabled])

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next && !disabled) {
          const saved = parseLocalDateTime(value)
          let initial = saved ?? new Date()
          if (!saved) {
            const lower = parseLocalDateTime(min ?? '')
            const upper = parseLocalDateTime(max ?? '')
            if (lower && initial < lower) initial = lower
            if (upper && initial > upper) initial = upper
          }
          choose(initial)
          setOpen(true)
        } else setOpen(false)
      }}
    >
      <DialogTrigger asChild>
        <button
          id={id}
          type="button"
          disabled={disabled}
          aria-required={required}
          aria-haspopup="dialog"
          aria-expanded={open}
          className={cn(
            'form-control flex h-10 w-full items-center justify-between gap-2 rounded-lg border border-input px-3 text-left text-sm transition-colors',
            className,
          )}
        >
          <span className={cn('tabular-nums', !value && 'text-muted-foreground')}>
            {value ? value.replace('T', ' ') : '选择日期与时间'}
          </span>
          <CalendarDays className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </DialogTrigger>
      <DialogContent
        className="max-w-sm rounded-xl p-5"
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          cells.current.get(localDateText(cursor))?.focus()
        }}
      >
        <DialogHeader>
          <DialogTitle>选择日期与时间</DialogTitle>
          <DialogDescription>
            按本地时区 {Intl.DateTimeFormat().resolvedOptions().timeZone} 设置。
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center justify-between">
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="上个月"
            onClick={() => setCursor(new Date(cursor.getFullYear(), cursor.getMonth() - 1, 1))}
          >
            <ChevronLeft />
          </Button>
          <span className="text-sm font-semibold tabular-nums">
            {cursor.getFullYear()} 年 {cursor.getMonth() + 1} 月
          </span>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label="下个月"
            onClick={() => setCursor(new Date(cursor.getFullYear(), cursor.getMonth() + 1, 1))}
          >
            <ChevronRight />
          </Button>
        </div>
        <div role="group" aria-label="日历" className="grid grid-cols-7 gap-1">
          {['一', '二', '三', '四', '五', '六', '日'].map((day) => (
            <span key={day} className="py-1 text-center text-xs text-muted-foreground">
              {day}
            </span>
          ))}
          {calendarDays(cursor).map((day) => {
            const key = localDateText(day)
            const selected = date === key
            return (
              <button
                key={key}
                ref={(node) => {
                  if (node) cells.current.set(key, node)
                  else cells.current.delete(key)
                }}
                type="button"
                aria-label={key}
                aria-pressed={selected}
                tabIndex={key === localDateText(cursor) ? 0 : -1}
                onClick={() => {
                  setDate(key)
                  setCursor(day)
                }}
                onKeyDown={(event) => {
                  const delta = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 }[
                    event.key
                  ]
                  if (delta !== undefined) {
                    event.preventDefault()
                    focusDay.current = true
                    setCursor(new Date(day.getFullYear(), day.getMonth(), day.getDate() + delta))
                  }
                }}
                className={cn(
                  'h-9 rounded-md text-sm tabular-nums transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-primary',
                  day.getMonth() !== cursor.getMonth() && 'text-muted-foreground/50',
                  selected && 'bg-primary text-primary-foreground hover:bg-primary/90',
                )}
              >
                {day.getDate()}
              </button>
            )
          })}
        </div>
        <div className="grid grid-cols-[1fr_1fr] gap-3 border-t border-border pt-4">
          <label className="flex flex-col gap-1.5 text-xs text-muted-foreground">
            日期
            <Input
              value={date}
              placeholder="YYYY-MM-DD"
              aria-label="日期，格式 YYYY-MM-DD"
              onChange={(event) => {
                setDate(event.target.value)
                const next = parseLocalDateTime(`${event.target.value}T${hour}:${minute}`)
                if (next) setCursor(next)
              }}
            />
          </label>
          <div className="flex flex-col gap-1.5">
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              <Clock3 className="size-3" />
              时间
            </span>
            <div className="flex items-center gap-1">
              <ChoiceSelect
                id={`${prefix}-hour`}
                label="小时"
                value={hour}
                onValueChange={setHour}
                options={hours}
              />
              <span>:</span>
              <ChoiceSelect
                id={`${prefix}-minute`}
                label="分钟"
                value={minute}
                onValueChange={setMinute}
                options={minutes}
              />
            </div>
          </div>
        </div>
        {!valid && (
          <p role="alert" className="text-xs text-destructive">
            {beforeMin
              ? `不能早于 ${min?.replace('T', ' ')}。`
              : afterMax
                ? `不能晚于 ${max?.replace('T', ' ')}。`
                : '请输入有效日期，格式为 YYYY-MM-DD。'}
          </p>
        )}
        <div className="flex gap-2">
          <Button size="sm" variant="ghost" onClick={() => choose(new Date())}>
            现在
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              const tomorrow = new Date()
              tomorrow.setDate(tomorrow.getDate() + 1)
              choose(tomorrow)
            }}
          >
            明天
          </Button>
        </div>
        <DialogFooter className="border-t border-border pt-3">
          {!required && (
            <Button
              className="mr-auto"
              variant="ghost"
              onClick={() => {
                onChange('')
                setOpen(false)
              }}
            >
              清除
            </Button>
          )}
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button
            disabled={!valid}
            onClick={() => {
              onChange(candidate)
              setOpen(false)
            }}
          >
            确定
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
