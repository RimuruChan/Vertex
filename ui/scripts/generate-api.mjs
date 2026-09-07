import { spawnSync } from 'node:child_process'
import { setTimeout } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

const cwd = fileURLToPath(new URL('..', import.meta.url))
const run = (entry, args) =>
  spawnSync(process.execPath, [fileURLToPath(new URL(entry, import.meta.url)), ...args], {
    cwd,
    encoding: 'utf8',
    windowsHide: true,
    maxBuffer: 4 << 20,
  })
const output = (result) => {
  process.stdout.write(result.stdout ?? '')
  process.stderr.write(result.stderr ?? '')
  if (result.error) console.error(result.error.message)
}

const generated = run('../node_modules/orval/dist/bin/orval.js', ['--config', 'orval.config.ts'])
output(generated)
if (generated.status !== 0) process.exit(generated.status ?? 1)

for (let attempt = 0; attempt < 3; attempt++) {
  const formatted = run('../node_modules/prettier/bin/prettier.cjs', [
    '--write',
    'src/generated/api',
    '--log-level',
    'warn',
  ])
  if (formatted.status === 0) process.exit(0)
  output(formatted)
  // Newly generated files can briefly be locked on Windows. Retry only I/O
  // failures, never syntax errors or an unsuccessful schema generator.
  const transient =
    /(?:UNKNOWN|EBUSY|EPERM|EACCES):/.test(formatted.stderr ?? '') &&
    !/(?:SyntaxError|ParseError):/.test(formatted.stderr ?? '')
  if (process.platform !== 'win32' || !transient || attempt === 2) {
    process.exit(formatted.status ?? 1)
  }
  console.warn('Generated-file I/O failed; retrying formatting shortly.')
  await setTimeout(1000 * (attempt + 1))
}
