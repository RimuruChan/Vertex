export const contestSections: Record<string, string> = {
  problems: 'problems',
  standings: 'rankboard',
  clarifications: 'clarifications',
  settings: 'settings',
  composition: 'composition',
  access: 'access',
}

export function contestSectionPath(ref: string, tab: string): string {
  const section = tab === 'rankboard' || tab === 'board' ? 'standings' : tab
  return `/contests/${ref}/${section}`
}
