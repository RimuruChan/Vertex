import type { ContestDraft } from './contest-form'

export type ContestPreset = 'icpc' | 'oi' | 'ioi' | 'leduo' | 'cf'

export function applyContestPreset(draft: ContestDraft, preset: ContestPreset): ContestDraft {
  return {
    ...draft,
    rule: preset,
    feedback:
      preset === 'oi'
        ? 'none'
        : preset === 'icpc'
          ? 'summary'
          : preset === 'cf'
            ? 'first_error'
            : 'full',
    penaltyMinutes: preset === 'icpc' ? 20 : 0,
    penalizeCompileError: preset === 'leduo' || preset === 'oi',
    rankboardVisible: true,
    submissionVisibility: preset === 'oi' ? 'own' : 'during',
    sourceCodeVisibility: 'after_end',
    frozenSubmissionVisibility: 'pending',
  }
}
