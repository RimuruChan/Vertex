import { describe, expect, it } from 'vitest'
import {
  afterStatementSave,
  discardStatementDraft,
  readStatementDraft,
  statementDraftKey,
  statementRecoveryState,
  writeStatementDraft,
  type StatementDraft,
} from './statement-draft-recovery'

function memoryStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
    removeItem: (key: string) => {
      values.delete(key)
    },
  }
}
const scope = { userId: 'alice', domainSlug: 'practice', problemId: '12', entryId: 'zh' }
const draft: StatementDraft = {
  schemaVersion: 1,
  inputVersion: 'input-1',
  text: '未保存题面',
  baselineHash: 'server-1',
  updatedAt: 1234,
}

describe('statement draft recovery', () => {
  it('isolates every user, domain, problem and statement language identity', () => {
    const storage = memoryStorage()
    writeStatementDraft(storage, statementDraftKey(scope), draft)
    for (const key of Object.keys(scope) as (keyof typeof scope)[]) {
      expect(
        readStatementDraft(storage, statementDraftKey({ ...scope, [key]: scope[key] + '-other' })),
      ).toBeUndefined()
    }
    expect(statementDraftKey({ ...scope, userId: 'a:b', domainSlug: 'c' })).not.toBe(
      statementDraftKey({ ...scope, userId: 'a', domainSlug: 'b:c' }),
    )
    expect(readStatementDraft(storage, statementDraftKey(scope))).toEqual(draft)
  })
  it('does not treat a changed server baseline as automatically recoverable', () => {
    expect(statementRecoveryState(draft, '服务器题面', 'server-1')).toBe('same-baseline')
    expect(statementRecoveryState(draft, '协作者修改', 'server-2')).toBe('changed-baseline')
    expect(statementRecoveryState(draft, draft.text, 'server-2')).toBe('already-saved')
  })
  it('cleans only the input version that was actually saved', () => {
    const storage = memoryStorage(),
      key = statementDraftKey(scope)
    writeStatementDraft(storage, key, draft)
    expect(afterStatementSave(draft, draft, 'server-2')).toBeUndefined()
    discardStatementDraft(storage, key, draft.inputVersion)
    expect(readStatementDraft(storage, key)).toBeUndefined()
  })
  it('keeps and rebases input made while an earlier save is pending', () => {
    const storage = memoryStorage(),
      key = statementDraftKey(scope)
    const newer = { ...draft, inputVersion: 'input-2', text: '保存期间继续输入', updatedAt: 1240 }
    writeStatementDraft(storage, key, newer)
    discardStatementDraft(storage, key, draft.inputVersion)
    expect(readStatementDraft(storage, key)).toEqual(newer)
    const remaining = afterStatementSave(newer, draft, 'server-2')!
    expect(remaining).toEqual({ ...newer, baselineHash: 'server-2' })
    writeStatementDraft(storage, key, remaining)
    expect(statementRecoveryState(readStatementDraft(storage, key)!, draft.text, 'server-2')).toBe(
      'same-baseline',
    )
  })
  it('does not clear a newer input version even if its text happens to match', () => {
    const newer = { ...draft, inputVersion: 'input-2' }
    expect(afterStatementSave(newer, draft, 'server-2')).toEqual({
      ...newer,
      baselineHash: 'server-2',
    })
  })
  it('does not silently replace the baseline of an unrelated recovered draft', () => {
    const unrelated = { ...draft, inputVersion: 'other', baselineHash: 'other-server' }
    expect(afterStatementSave(unrelated, draft, 'server-2')).toBe(unrelated)
  })
  it('ignores invalid records without breaking server editing', () => {
    const storage = memoryStorage(),
      key = statementDraftKey(scope)
    for (const invalid of [
      '{',
      'null',
      '{}',
      JSON.stringify({ ...draft, schemaVersion: 2 }),
      JSON.stringify({ ...draft, updatedAt: 'yesterday' }),
    ]) {
      storage.setItem(key, invalid)
      expect(readStatementDraft(storage, key)).toBeUndefined()
    }
  })
  it('degrades safely when session storage is blocked or full', () => {
    const blocked = {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('full')
      },
      removeItem: () => {
        throw new Error('blocked')
      },
    }
    expect(readStatementDraft(blocked, 'key')).toBeUndefined()
    expect(writeStatementDraft(blocked, 'key', draft)).toBe(false)
    expect(writeStatementDraft(undefined, 'key', draft)).toBe(false)
    expect(() => discardStatementDraft(blocked, 'key', draft.inputVersion)).not.toThrow()
  })
})
