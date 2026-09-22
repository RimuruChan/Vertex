export type StatementDraft = {
  schemaVersion: 1
  inputVersion: string
  text: string
  baselineHash: string
  updatedAt: number
}

type DraftStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> | undefined

export function statementDraftKey(scope: {
  userId: string
  domainSlug: string
  problemId: string
  entryId: string
}) {
  return `vertex:statement-draft:v1:${JSON.stringify([scope.userId, scope.domainSlug, scope.problemId, scope.entryId])}`
}

export function readStatementDraft(storage: DraftStorage, key: string): StatementDraft | undefined {
  try {
    const raw = storage?.getItem(key)
    if (!raw) return
    const value = JSON.parse(raw) as Partial<StatementDraft> | null
    if (
      value?.schemaVersion === 1 &&
      typeof value.inputVersion === 'string' &&
      value.inputVersion.length > 0 &&
      typeof value.text === 'string' &&
      typeof value.baselineHash === 'string' &&
      value.baselineHash.length > 0 &&
      typeof value.updatedAt === 'number' &&
      Number.isFinite(value.updatedAt) &&
      value.updatedAt >= 0
    )
      return value as StatementDraft
  } catch {
    // Session storage may be unavailable, full, or contain an older/corrupt record.
  }
}

export function writeStatementDraft(storage: DraftStorage, key: string, draft: StatementDraft) {
  try {
    if (!storage) return false
    storage.setItem(key, JSON.stringify(draft))
    return true
  } catch {
    return false
  }
}

/** Never delete a newer input version when an earlier save or discard completes. */
export function discardStatementDraft(storage: DraftStorage, key: string, inputVersion: string) {
  try {
    if (readStatementDraft(storage, key)?.inputVersion === inputVersion) storage?.removeItem(key)
  } catch {
    // Storage failures must not prevent editing or saving to the server.
  }
}

export function statementRecoveryState(
  draft: StatementDraft,
  serverText: string,
  serverHash: string,
) {
  if (draft.text === serverText) return 'already-saved'
  return draft.baselineHash === serverHash ? 'same-baseline' : 'changed-baseline'
}

export function afterStatementSave(
  latest: StatementDraft | undefined,
  saved: StatementDraft | undefined,
  serverHash: string,
): StatementDraft | undefined {
  if (!latest || !saved) return latest
  if (latest.inputVersion === saved.inputVersion) return undefined
  // New keystrokes made during this save now sit on the successfully saved text.
  return latest.baselineHash === saved.baselineHash
    ? { ...latest, baselineHash: serverHash }
    : latest
}
