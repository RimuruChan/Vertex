import { useLayoutEffect, useRef, useState } from 'react'
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
import { cn } from '@/lib/utils'
import { calendarDays, localDateText, parseLocalDateTime } from '@/lib/date-time'

export function DateTimePicker({
  id,
  value,
  onChange,
  disabled = false,
  required = false,
  min,
  max,
  className,
  ariaLabel,
  placeholder = '选择日期与时间',
}: {
  id?: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  required?: boolean
  min?: string
  max?: string
  className?: string
  ariaLabel?: string
  placeholder?: string
}) {
  const [open, setOpen] = useState(false)
  const [date, setDate] = useState('')
  const [hour, setHour] = useState('09')
  const [minute, setMinute] = useState('00')
  const [cursor, setCursor] = useState(new Date())
  const focusDay = useRef(false)
  const cells = useRef(new Map<string, HTMLButtonElement>())
  const candidate =
    hour && minute ? `${date}T${hour.padStart(2, '0')}:${minute.padStart(2, '0')}` : ''
  const parsed = parseLocalDateTime(candidate)
  const lower = parseLocalDateTime(min ?? '')
  const upper = parseLocalDateTime(max ?? '')
  const beforeMin = parsed && lower && parsed < lower
  const afterMax = parsed && upper && parsed > upper
  const valid = !!parsed && !beforeMin && !afterMax
  const today = localDateText(new Date())
  function commit() {
    if (!valid) return
    onChange(candidate)
    setOpen(false)
  }
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
          if (lower && initial < lower) initial = lower
          if (upper && initial > upper) initial = upper
          choose(initial)
          setOpen(true)
        } else setOpen(false)
      }}
    >
      <DialogTrigger asChild>
        <button
          id={id}
          aria-label={ariaLabel}
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
          <span className={cn('truncate tabular-nums', !value && 'text-muted-foreground')}>
            {value ? value.replace('T', ' ') : placeholder}
          </span>
          <CalendarDays className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </DialogTrigger>
      <DialogContent
        className="max-w-[600px] gap-0 rounded-2xl p-0"
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          cells.current.get(localDateText(cursor))?.focus()
        }}
        onKeyDown={(event) => {
          if (
            event.key === 'Enter' &&
            event.target instanceof HTMLInputElement &&
            !event.nativeEvent.isComposing
          ) {
            event.preventDefault()
            event.stopPropagation()
            commit()
          }
        }}
      >
        <DialogHeader className="border-b border-border px-5 py-4 pr-16">
          <DialogTitle>选择日期与时间</DialogTitle>
          <DialogDescription className="text-xs">
            本地时区 · {Intl.DateTimeFormat().resolvedOptions().timeZone}
          </DialogDescription>
        </DialogHeader>
        <div className="grid sm:grid-cols-[minmax(0,1fr)_208px]">
          <div className="min-w-0 p-5">
            <div className="mb-3 flex items-center justify-between">
              <Button
                type="button"
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
                type="button"
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
                const unavailable = !!(
                  (lower && key < localDateText(lower)) ||
                  (upper && key > localDateText(upper))
                )
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
                    aria-current={key === today ? 'date' : undefined}
                    disabled={unavailable}
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
                        let next = new Date(
                          day.getFullYear(),
                          day.getMonth(),
                          day.getDate() + delta,
                        )
                        if (lower && localDateText(next) < localDateText(lower)) next = lower
                        if (upper && localDateText(next) > localDateText(upper)) next = upper
                        setCursor(next)
                      }
                    }}
                    className={cn(
                      'relative h-10 rounded-lg text-sm tabular-nums transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-primary disabled:pointer-events-none disabled:opacity-25',
                      day.getMonth() !== cursor.getMonth() && 'text-muted-foreground/50',
                      selected && 'bg-primary text-primary-foreground hover:bg-primary/90',
                      key === today &&
                        !selected &&
                        'font-semibold text-primary after:absolute after:bottom-1 after:left-1/2 after:size-1 after:-translate-x-1/2 after:rounded-full after:bg-primary',
                    )}
                  >
                    {day.getDate()}
                  </button>
                )
              })}
            </div>
          </div>
          <div className="flex min-w-0 flex-col gap-5 border-t border-border bg-muted/20 p-5 sm:border-l sm:border-t-0">
            <label className="flex flex-col gap-2 text-xs text-muted-foreground">
              日期
              <Input
                value={date}
                className="tabular-nums"
                placeholder="YYYY-MM-DD"
                aria-label="日期，格式 YYYY-MM-DD"
                onChange={(event) => {
                  setDate(event.target.value)
                  const next = parseLocalDateTime(`${event.target.value}T00:00`)
                  if (next) setCursor(next)
                }}
              />
            </label>
            <div className="flex flex-col gap-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Clock3 className="size-3" />
                时间 · 24 小时制
              </span>
              <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2">
                <Input
                  aria-label="小时"
                  inputMode="numeric"
                  maxLength={2}
                  className="h-12 px-1 text-center text-xl font-medium tabular-nums"
                  value={hour}
                  onChange={(event) => {
                    if (/^\d{0,2}$/.test(event.target.value)) setHour(event.target.value)
                  }}
                  onFocus={(event) => event.target.select()}
                  onBlur={() => {
                    if (hour) setHour(hour.padStart(2, '0'))
                  }}
                />
                <span className="text-muted-foreground">:</span>
                <Input
                  aria-label="分钟"
                  inputMode="numeric"
                  maxLength={2}
                  className="h-12 px-1 text-center text-xl font-medium tabular-nums"
                  value={minute}
                  onChange={(event) => {
                    if (/^\d{0,2}$/.test(event.target.value)) setMinute(event.target.value)
                  }}
                  onFocus={(event) => event.target.select()}
                  onBlur={() => {
                    if (minute) setMinute(minute.padStart(2, '0'))
                  }}
                />
              </div>
            </div>
            {!valid && (
              <p role="alert" className="text-xs text-destructive">
                {beforeMin
                  ? `不能早于 ${min?.replace('T', ' ')}。`
                  : afterMax
                    ? `不能晚于 ${max?.replace('T', ' ')}。`
                    : '请填写有效日期与时间，小时为 00–23，分钟为 00–59。'}
              </p>
            )}
            {valid && (
              <div className="mt-auto border-t border-border/60 pt-4" aria-live="polite">
                <p className="text-xs text-muted-foreground">已选择</p>
                <p className="mt-1 text-sm font-medium tabular-nums">
                  {candidate.replace('T', ' ')}
                </p>
              </div>
            )}
          </div>
        </div>
        <DialogFooter className="border-t border-border px-5 py-4">
          {!required && (
            <Button
              type="button"
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
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button type="button" disabled={!valid} onClick={commit}>
            确认时间
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
