import { useState } from 'react'
import { ChevronDown, Monitor, Moon, Sun, SunMoon } from 'lucide-react'
import { useTheme } from '@/components/ThemeProvider'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const options = [
  { value: 'light', label: '浅色', icon: Sun },
  { value: 'dark', label: '深色', icon: Moon },
  { value: 'system', label: '跟随系统', icon: Monitor },
] as const

function AppearanceOptions({ inset = false }: { inset?: boolean }) {
  const { theme, setTheme } = useTheme()
  return (
    <DropdownMenuRadioGroup
      value={theme}
      onValueChange={(value) => {
        if (value === 'light' || value === 'dark' || value === 'system') setTheme(value)
      }}
      aria-label="外观模式"
    >
      {options.map(({ value, label, icon: Icon }) => (
        <DropdownMenuRadioItem key={value} value={value} className={inset ? 'pl-6' : undefined}>
          <Icon />
          {label}
        </DropdownMenuRadioItem>
      ))}
    </DropdownMenuRadioGroup>
  )
}

export function AppearanceSection() {
  const [expanded, setExpanded] = useState(false)
  return (
    <>
      <DropdownMenuItem
        aria-expanded={expanded}
        onSelect={(event) => {
          event.preventDefault()
          setExpanded((value) => !value)
        }}
      >
        <SunMoon />
        外观
        <ChevronDown
          className={`ml-auto size-3.5 transition-transform ${expanded ? 'rotate-180' : ''}`}
        />
      </DropdownMenuItem>
      {expanded && <AppearanceOptions inset />}
    </>
  )
}

export default function AppearanceMenu() {
  const { theme } = useTheme()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="text-muted-foreground hover:text-foreground"
          aria-label="切换外观"
          title={`外观：${options.find((option) => option.value === theme)?.label}`}
        >
          <SunMoon className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="end" collisionPadding={12}>
        <DropdownMenuLabel>外观</DropdownMenuLabel>
        <AppearanceOptions />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
