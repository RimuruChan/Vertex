import { spawnSync } from 'node:child_process'

const paths = process.argv.slice(2)

if (paths.length === 0) {
  console.error('usage: node check-generated-clean.mjs <path> [path...]')
  process.exit(2)
}

const result = spawnSync(
  'git',
  ['status', '--porcelain=v1', '--untracked-files=all', '--', ...paths],
  { encoding: 'utf8' },
)

if (result.error) {
  console.error(`failed to run git status: ${result.error.message}`)
  process.exit(2)
}
if (result.status !== 0) {
  if (result.stderr) process.stderr.write(result.stderr)
  process.exit(result.status ?? 2)
}

const changes = result.stdout.trimEnd()
if (changes) {
  console.error('generated files are not committed or are out of date:')
  console.error(changes)
  process.exit(1)
}
