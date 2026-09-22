import type {
  DomainCheckRun,
  DomainCommitRelease,
  DomainMaterialInspection,
} from '@/generated/api/model'

export function checkMatchesRelease(check: DomainCheckRun, inspection?: DomainMaterialInspection) {
  return Boolean(
    inspection?.dataHash &&
    inspection.policyVersion &&
    check.dataHash === inspection.dataHash &&
    check.policyVersion === inspection.policyVersion,
  )
}

export function selectionIsCurrentRelease(
  release: DomainCommitRelease | undefined,
  currentVersion: number,
  revision: number,
  checkId: string | undefined,
  language: string,
) {
  return Boolean(
    release &&
    release.version === currentVersion &&
    release.revision === revision &&
    checkId &&
    release.checkId === checkId &&
    release.language === language,
  )
}
