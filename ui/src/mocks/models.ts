// Fixture bookkeeping tracks allocated numbers; the wire serializer exposes one resource id.
export * from '@/generated/api/model'
import type * as Wire from '@/generated/api/model'
type MockModel<T> = T extends readonly (infer U)[]
  ? MockModel<U>[]
  : T extends object
    ? { [K in keyof T]: MockModel<T[K]> } & {
        publicId?: string
        problemPublicId?: string
        contestPublicId?: string
      }
    : T
export type DtoAccountResponse = MockModel<Wire.DtoAccountResponse>
export type DtoActivityDayResponse = MockModel<Wire.DtoActivityDayResponse>
export type DtoAnnouncementResponse = MockModel<Wire.DtoAnnouncementResponse>
export type DtoAuthResponse = MockModel<Wire.DtoAuthResponse>
export type DtoCaseResultResponse = MockModel<Wire.DtoCaseResultResponse>
export type DtoClarificationResponse = MockModel<Wire.DtoClarificationResponse>
export type DtoContestDetailsResponse = MockModel<Wire.DtoContestDetailsResponse>
export type DtoContestGrantResponse = MockModel<Wire.DtoContestGrantResponse>
export type DtoContestProblemDetailResponse = MockModel<Wire.DtoContestProblemDetailResponse>
export type DtoContestProblemResponse = MockModel<Wire.DtoContestProblemResponse>
export type DtoContestResponse = MockModel<Wire.DtoContestResponse>
export type DtoContestStaffResponse = MockModel<Wire.DtoContestStaffResponse>
export type DtoCopyOriginResponse = MockModel<Wire.DtoCopyOriginResponse>
export type DtoCopyResponse = MockModel<Wire.DtoCopyResponse>
export type DtoDifficultyProgressResponse = MockModel<Wire.DtoDifficultyProgressResponse>
export type DtoDiscussionResponse = MockModel<Wire.DtoDiscussionResponse>
export type DtoDiscussionThreadResponse = MockModel<Wire.DtoDiscussionThreadResponse>
export type DtoDomainResponse = MockModel<Wire.DtoDomainResponse>
export type DtoEditorialResponse = MockModel<Wire.DtoEditorialResponse>
export type DtoEditorialSummaryResponse = MockModel<Wire.DtoEditorialSummaryResponse>
export type DtoEditorialVoteResponse = MockModel<Wire.DtoEditorialVoteResponse>
export type DtoGroupMemberResponse = MockModel<Wire.DtoGroupMemberResponse>
export type DtoGroupResponse = MockModel<Wire.DtoGroupResponse>
export type DtoJobResponse = MockModel<Wire.DtoJobResponse>
export type DtoMemberResponse = MockModel<Wire.DtoMemberResponse>
export type DtoPermissionResponse = MockModel<Wire.DtoPermissionResponse>
export type DtoProblemGrantResponse = MockModel<Wire.DtoProblemGrantResponse>
export type DtoProblemOriginResponse = MockModel<Wire.DtoProblemOriginResponse>
export type DtoProblemResponse = MockModel<Wire.DtoProblemResponse>
export type DtoProfileResponse = MockModel<Wire.DtoProfileResponse>
export type DtoRankboardCellResponse = MockModel<Wire.DtoRankboardCellResponse>
export type DtoRankboardProblemResponse = MockModel<Wire.DtoRankboardProblemResponse>
export type DtoRankboardResponse = MockModel<Wire.DtoRankboardResponse>
export type DtoRankboardRowResponse = MockModel<Wire.DtoRankboardRowResponse>
export type DtoRegistrationResponse = MockModel<Wire.DtoRegistrationResponse>
export type DtoRejudgingChangeResponse = MockModel<Wire.DtoRejudgingChangeResponse>
export type DtoRejudgingResponse = MockModel<Wire.DtoRejudgingResponse>
export type DtoRoleResponse = MockModel<Wire.DtoRoleResponse>
export type DtoSetAccessResponse = MockModel<Wire.DtoSetAccessResponse>
export type DtoSetItemResponse = MockModel<Wire.DtoSetItemResponse>
export type DtoSetResponse = MockModel<Wire.DtoSetResponse>
export type DtoSolutionOutcomeResponse = MockModel<Wire.DtoSolutionOutcomeResponse>
export type DtoStatsResponse = MockModel<Wire.DtoStatsResponse>
export type DtoSubmissionProgressResponse = MockModel<Wire.DtoSubmissionProgressResponse>
export type DtoSubmissionResponse = MockModel<Wire.DtoSubmissionResponse>
export type DtoTagCatalogResponse = MockModel<Wire.DtoTagCatalogResponse>
export type DtoTagResponse = MockModel<Wire.DtoTagResponse>
export type DtoTestOutcomeResponse = MockModel<Wire.DtoTestOutcomeResponse>
export type DtoUserResponse = MockModel<Wire.DtoUserResponse>
export type DtoVerdictCountResponse = MockModel<Wire.DtoVerdictCountResponse>
