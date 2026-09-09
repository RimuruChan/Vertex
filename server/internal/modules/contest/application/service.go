package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (s *Service) UseProblemVersion(ctx context.Context, contestID, problemID string, version, expected int) error {
	if version <= 0 || expected <= 0 {
		return contestdomain.Invalid("positive version and expected version are required")
	}
	access, err := s.repository.Access(ctx, contestID, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Rejudge {
		return contestdomain.ErrForbidden
	}
	item, err := s.repository.Problem(ctx, contestID, problemID)
	if err != nil {
		return err
	}
	return s.repository.UseProblemVersion(ctx, contestID, item.ProblemID, version, expected)
}

type PasswordManager interface {
	HashPassword(password string) (string, error)
	CheckPassword(hash, password string) bool
}

type Service struct {
	repository     contestdomain.Repository
	clarifications contestdomain.ClarificationRepository
	passwords      PasswordManager
	now            func() time.Time
}

// NewService wires the contest domain. The clarification repository is
// optional so a deployment can run without the question channel; every
// clarification entry point degrades to "not found" when it is absent.
func NewService(repository contestdomain.Repository, passwords PasswordManager) *Service {
	service := &Service{repository: repository, passwords: passwords, now: time.Now}
	if clarifications, ok := repository.(contestdomain.ClarificationRepository); ok {
		service.clarifications = clarifications
	}
	return service
}

func (s *Service) List(ctx context.Context, limit, offset int, admin bool, keyword ...string) ([]contestdomain.Contest, int, error) {
	if len(keyword) > 0 && len(keyword[0]) > 200 {
		return nil, 0, contestdomain.Invalid("search keyword must be at most 200 bytes")
	}
	if admin {
		return s.repository.ListAdmin(ctx, limit, offset, keyword...)
	}
	return s.repository.List(ctx, limit, offset, keyword...)
}

// Viewer resolves the caller's contest-scoped rights once, so every other
// entry point can reason about a single value instead of re-deriving roles.
func (s *Service) Viewer(ctx context.Context, contestID string, userID string) (contestdomain.Viewer, error) {
	access, err := s.repository.Access(ctx, contestID, userID)
	if err != nil {
		return contestdomain.Viewer{}, err
	}
	viewer := contestdomain.Viewer{UserID: userID, Access: &access, Role: "user"}
	if access.Scope.SiteAdmin {
		viewer.Role = "admin"
	}
	if access.Permissions.Rejudge {
		viewer.Staff = contestdomain.StaffJury
	} else if access.Permissions.ViewJury {
		viewer.Staff = contestdomain.StaffObserver
	}
	return viewer, nil
}

func (s *Service) Details(ctx context.Context, id string, userID string, role string) (*contestdomain.Details, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	viewer, registered, err := s.resolveViewerAccess(ctx, item, userID, role)
	if err != nil {
		return nil, err
	}
	item.Permissions = viewer.Access.Permissions
	privileged := viewer.CanPreview()
	if !privileged && item.Visibility == "public" && userID != "" {
		registered, err = s.isParticipant(ctx, item.ID, userID)
		if err != nil {
			return nil, err
		}
	}

	problems, err := s.repository.Problems(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if !privileged && item.Visibility == "password" && !registered {
		problems = nil
	}
	if !privileged {
		// A contest may include problems that are not published on their own;
		// contestants still need to see them, but only once the contest starts.
		visible := make([]contestdomain.Problem, 0, len(problems))
		for _, problem := range problems {
			if problem.Visibility == "public" || (registered && !s.now().Before(item.BeginAt)) {
				visible = append(visible, problem)
			}
		}
		problems = visible
	}
	if !item.ShowProblemMetadata && s.now().Before(item.EndAt) {
		// Copy before redacting: repositories may reuse their loaded values.
		problems = append([]contestdomain.Problem(nil), problems...)
		for i := range problems {
			problems[i].Difficulty = 0
			problems[i].Tags = nil
		}
	}
	if userID != "" {
		statuses, err := s.repository.ProblemStatuses(ctx, item.ID, userID)
		if err != nil {
			return nil, err
		}
		problems = append([]contestdomain.Problem(nil), problems...)
		for i := range problems {
			problems[i].UserStatus = ownProblemStatus(statuses[problems[i].ProblemID].UserStatus, item, viewer, s.now())
			problems[i].LastSubmissionID = statuses[problems[i].ProblemID].LastSubmissionID
		}
	}
	return &contestdomain.Details{Contest: item, Problems: problems, Staff: viewer.Staff}, nil
}

// Problem returns a full statement through the contest access boundary.
// Ordinary users must be registered and may only open it once the contest has
// started. Staff and the contest creator may preview it before the start.
func (s *Service) Problem(
	ctx context.Context, contestID, problemID, userID, role string,
) (*contestdomain.ProblemDetail, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}
	viewer, registered, err := s.resolveViewerAccess(ctx, item, userID, role)
	if err != nil {
		return nil, err
	}
	if !viewer.CanPreview() {
		if s.now().Before(item.BeginAt) {
			return nil, contestdomain.ErrNotFound
		}
		if item.Visibility == "public" {
			registered, err = s.isParticipant(ctx, item.ID, userID)
			if err != nil {
				return nil, err
			}
		}
		if !registered {
			return nil, contestdomain.ErrRegistrationNeeded
		}
	}
	detail, err := s.repository.Problem(ctx, item.ID, problemID)
	if errors.Is(err, contestdomain.ErrProblemNotInContest) {
		return nil, contestdomain.ErrNotFound
	}
	if err == nil && detail != nil && !item.ShowProblemMetadata && s.now().Before(item.EndAt) {
		copy := *detail
		copy.Difficulty = 0
		copy.Tags = nil
		detail = &copy
	}
	if err == nil && detail != nil && userID != "" {
		statuses, readErr := s.repository.ProblemStatuses(ctx, item.ID, userID)
		if readErr != nil {
			return nil, readErr
		}
		result := *detail
		result.UserStatus = ownProblemStatus(statuses[detail.ProblemID].UserStatus, item, viewer, s.now())
		result.LastSubmissionID = statuses[detail.ProblemID].LastSubmissionID
		detail = &result
	}
	return detail, err
}

func ownProblemStatus(status string, contest *contestdomain.Contest, viewer contestdomain.Viewer, now time.Time) string {
	if status == "" {
		return "none"
	}
	if !viewer.IsStaff() && contest.FeedbackFor(now) == contestdomain.FeedbackNone {
		return "submitted"
	}
	return status
}

// resolveViewerAccess applies the shared contest visibility boundary. It also
// resolves password-contest participation once for callers that need to
// decide whether protected content may be returned.
func (s *Service) resolveViewerAccess(
	ctx context.Context, item *contestdomain.Contest, userID, role string,
) (contestdomain.Viewer, bool, error) {
	viewer, err := s.Viewer(ctx, item.ID, userID)
	if err != nil {
		return viewer, false, err
	}
	if !viewer.Access.Permissions.View {
		return viewer, false, contestdomain.ErrNotFound
	}
	switch item.Visibility {
	case "public":
		return viewer, false, nil
	case "private":
		return viewer, viewer.Access.Registered, nil
	case "password":
		if viewer.IsStaff() {
			return viewer, false, nil
		}
	default:
		return viewer, false, contestdomain.ErrNotFound
	}
	registered, err := s.isParticipant(ctx, item.ID, userID)
	return viewer, registered, err
}

func (s *Service) Create(ctx context.Context, createdBy string, input contestdomain.UpsertInput) (*contestdomain.Contest, error) {
	persisted, err := prepareInput(input, "", s.passwords)
	if err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, createdBy, persisted)
}

func (s *Service) Update(ctx context.Context, id string, input contestdomain.UpsertInput) (*contestdomain.Contest, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Admission == "" {
		input.Admission = current.Admission
	}
	if input.Visibility == "" {
		input.Visibility = current.Visibility
	}
	persisted, err := prepareInput(input, current.PasswordHash, s.passwords)
	if err != nil {
		return nil, err
	}
	return s.repository.Update(ctx, id, persisted)
}

var problemLabelPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,7}$`)

// SetProblems preserves explicit labels; only omitted labels follow position.
func (s *Service) SetProblems(ctx context.Context, contestID string, entries []contestdomain.ProblemEntry) error {
	prepared := make([]contestdomain.ProblemEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	labels := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		entry.ProblemID = strings.TrimSpace(entry.ProblemID)
		if entry.ProblemID == "" {
			return contestdomain.Invalid("problem ID is required")
		}
		if _, duplicate := seen[entry.ProblemID]; duplicate {
			return contestdomain.Invalid("a problem may only appear once in a contest")
		}
		seen[entry.ProblemID] = struct{}{}

		entry.Label = strings.TrimSpace(entry.Label)
		if entry.Label == "" {
			entry.Label = defaultLabel(index)
		}
		if !problemLabelPattern.MatchString(entry.Label) {
			return contestdomain.Invalid("problem label must start with a letter and contain at most 8 letters or digits")
		}
		if _, duplicate := labels[entry.Label]; duplicate {
			return contestdomain.Invalid("problem labels must be unique")
		}
		labels[entry.Label] = struct{}{}
		entry.Color = strings.TrimSpace(entry.Color)
		if len(entry.Color) > 32 {
			return contestdomain.Invalid("problem colour must be at most 32 characters")
		}
		if entry.Points <= 0 {
			entry.Points = 100
		}
		if entry.Points > 100000 {
			return contestdomain.Invalid("problem points must be at most 100000")
		}
		prepared = append(prepared, entry)
	}
	return s.repository.SetProblems(ctx, contestID, prepared)
}

// defaultLabel numbers problems A…Z, then AA, AB… for very large contests.
func defaultLabel(index int) string {
	label := ""
	for {
		label = string(rune('A'+index%26)) + label
		index = index/26 - 1
		if index < 0 {
			return label
		}
	}
}

func (s *Service) Register(ctx context.Context, contestID, userID, role, password string) error {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return err
	}
	viewer, err := s.Viewer(ctx, contestID, userID)
	if err != nil {
		return err
	}
	if !viewer.Access.Permissions.View {
		return contestdomain.ErrNotFound
	}
	registered, err := s.repository.IsParticipant(ctx, item.ID, userID)
	if err != nil || registered {
		return err
	}
	if err := contestdomain.RegistrationError(item.AllowSelfRegistration, item.AllowLateRegistration, item.BeginAt, item.EndAt, s.now()); err != nil {
		return err
	}
	if !viewer.Access.Permissions.Register {
		return contestdomain.ErrForbidden
	}
	if item.Visibility == "password" && !s.passwords.CheckPassword(item.PasswordHash, password) {
		return contestdomain.ErrInvalidPassword
	}
	return s.repository.Register(ctx, item.ID, userID, item.PasswordHash)
}

func (s *Service) Registration(ctx context.Context, contestID, userID, role string) (bool, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return false, err
	}
	viewer, err := s.Viewer(ctx, contestID, userID)
	if err != nil {
		return false, err
	}
	if !viewer.Access.Permissions.View {
		return false, contestdomain.ErrNotFound
	}
	return s.repository.IsParticipant(ctx, item.ID, userID)
}

// Rankboard returns the scoreboard for one viewer. Jury and observers always
// receive the unfrozen board; everyone else receives the frozen view while the
// freeze is in effect.
func (s *Service) Rankboard(ctx context.Context, contestID, userID, role string, juryView bool) (*contestdomain.Rankboard, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}
	viewer, err := s.Viewer(ctx, contestID, userID)
	if err != nil {
		return nil, err
	}
	if !viewer.Access.Permissions.View {
		return nil, contestdomain.ErrNotFound
	}
	// A public scoreboard must not reveal results withheld by the feedback policy.
	if item.FeedbackFor(s.now()) == contestdomain.FeedbackNone && !(viewer.IsStaff() && juryView) {
		return nil, contestdomain.ErrRankboardHidden
	}
	if !viewer.IsStaff() {
		if s.now().Before(item.BeginAt) {
			return nil, contestdomain.ErrNotFound
		}
		if item.Visibility == "password" {
			registered, err := s.isParticipant(ctx, item.ID, userID)
			if err != nil {
				return nil, err
			}
			if !registered {
				return nil, contestdomain.ErrRegistrationNeeded
			}
		}
		if !item.RankboardVisible {
			return nil, contestdomain.ErrRankboardHidden
		}
	}

	// During a freeze only an explicit staff view bypasses the snapshot.
	// After public unfreeze everyone reads full cells, without acquiring jury rights.
	frozen := item.Frozen(s.now())
	full := !frozen || (viewer.IsStaff() && juryView)
	board, err := s.repository.Rankboard(ctx, item.ID, full)
	if err != nil {
		return nil, err
	}
	board.Frozen = !full
	board.FullResults = full
	board.FrozenAt = item.FreezeAt
	board.UnfreezeAt = item.UnfreezeAt
	board.JuryView = viewer.IsStaff() && full
	return board, nil
}

func (s *Service) ValidateSubmission(ctx context.Context, contestID, userID, role, problemID string) error {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return err
	}
	if !item.Running(s.now()) {
		return contestdomain.ErrNotActive
	}
	viewer, registered, err := s.resolveViewerAccess(ctx, item, userID, role)
	if err != nil {
		return err
	}
	if !viewer.Access.Permissions.Submit {
		return contestdomain.ErrNotParticipant
	}
	if !viewer.IsStaff() {
		if item.Visibility == "public" {
			registered, err = s.isParticipant(ctx, item.ID, userID)
			if err != nil {
				return err
			}
		}
		if !registered {
			return contestdomain.ErrNotParticipant
		}
	}
	hasProblem, err := s.repository.HasProblem(ctx, item.ID, problemID)
	if err != nil {
		return err
	}
	if !hasProblem {
		return contestdomain.ErrProblemNotInContest
	}
	return nil
}

// Feedback reports the feedback level that applies to a contestant's own
// submission in this contest right now. Staff always see everything.
func (s *Service) Feedback(ctx context.Context, contestID string, viewer contestdomain.Viewer) (string, error) {
	if viewer.IsStaff() {
		return contestdomain.FeedbackFull, nil
	}
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return contestdomain.FeedbackFull, err
	}
	return item.FeedbackFor(s.now()), nil
}

// ---------- staff ----------

func (s *Service) ListStaff(ctx context.Context, contestID string) ([]contestdomain.Staff, error) {
	if _, err := s.RequireStaff(ctx, contestID, tenancydomain.ActorID(ctx), ""); err != nil {
		return nil, err
	}
	return s.repository.ListStaff(ctx, contestID)
}

func (s *Service) AddStaff(ctx context.Context, contestID, username, role string) (*contestdomain.Staff, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, contestdomain.Invalid("username is required")
	}
	if role != contestdomain.StaffJury && role != contestdomain.StaffObserver {
		return nil, contestdomain.Invalid("staff role must be jury or observer")
	}
	return s.repository.AddStaff(ctx, contestID, username, role)
}

func (s *Service) RemoveStaff(ctx context.Context, contestID, userID string) error {
	return s.repository.RemoveStaff(ctx, contestID, userID)
}

// RequireJury resolves the viewer and rejects callers without jury rights.
func (s *Service) RequireJury(ctx context.Context, contestID, userID, role string) (contestdomain.Viewer, error) {
	viewer, err := s.Viewer(ctx, contestID, userID)
	if err != nil {
		return viewer, err
	}
	if !viewer.IsJury() {
		return viewer, contestdomain.ErrForbidden
	}
	return viewer, nil
}

// RequireStaff resolves the viewer and rejects callers without jury or
// observer rights.
func (s *Service) RequireStaff(ctx context.Context, contestID, userID, role string) (contestdomain.Viewer, error) {
	viewer, err := s.Viewer(ctx, contestID, userID)
	if err != nil {
		return viewer, err
	}
	if !viewer.IsStaff() {
		return viewer, contestdomain.ErrForbidden
	}
	return viewer, nil
}

func (s *Service) Get(ctx context.Context, id string) (*contestdomain.Contest, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) Grants(ctx context.Context, id string) ([]contestdomain.AccessGrant, error) {
	return s.repository.Grants(ctx, id)
}
func (s *Service) SetGrant(ctx context.Context, id string, input contestdomain.GrantInput) error {
	input.Username, input.Group = strings.TrimSpace(input.Username), strings.TrimSpace(input.Group)
	if (input.Username == "") == (input.Group == "") {
		return contestdomain.Invalid("select exactly one domain member or group")
	}
	if !contestdomain.ValidAccessRole(input.Role) {
		return contestdomain.Invalid("unknown contest access role")
	}
	return s.repository.SetGrant(ctx, id, input)
}
func (s *Service) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	if grantID <= 0 {
		return contestdomain.Invalid("grant ID must be positive")
	}
	return s.repository.RemoveGrant(ctx, id, grantID)
}
func (s *Service) Transfer(ctx context.Context, id, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return contestdomain.Invalid("new owner is required")
	}
	return s.repository.Transfer(ctx, id, username)
}
func (s *Service) Delete(ctx context.Context, id string) error { return s.repository.Delete(ctx, id) }
func (s *Service) RequireManageAccess(ctx context.Context, id, userID string) error {
	access, err := s.repository.Access(ctx, id, userID)
	if err != nil {
		return err
	}
	if !access.Permissions.ManageAccess {
		return contestdomain.ErrForbidden
	}
	return nil
}

func (s *Service) isParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	return s.repository.IsParticipant(ctx, contestID, userID)
}

func prepareInput(input contestdomain.UpsertInput, existingPasswordHash string, passwords PasswordManager) (*contestdomain.PersistInput, error) {
	if input.Admission == "" {
		input.Admission = contestdomain.AdmissionMembers
	}
	if input.Admission != contestdomain.AdmissionMembers && input.Admission != contestdomain.AdmissionRestricted {
		return nil, contestdomain.Invalid("admission must be members or restricted")
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, contestdomain.Invalid("title required")
	}
	if input.SubmissionVisibility == "" {
		input.SubmissionVisibility = "own"
	}
	if input.SourceCodeVisibility == "" {
		input.SourceCodeVisibility = "own"
	}
	if input.FrozenSubmissionVisibility == "" {
		input.FrozenSubmissionVisibility = "pending"
	}
	if input.SubmissionVisibility != "own" && input.SubmissionVisibility != "after_end" && input.SubmissionVisibility != "during" {
		return nil, contestdomain.Invalid("invalid submission visibility")
	}
	if input.SourceCodeVisibility != "own" && input.SourceCodeVisibility != "after_end" {
		return nil, contestdomain.Invalid("invalid source code visibility")
	}
	if input.FrozenSubmissionVisibility != "hidden" && input.FrozenSubmissionVisibility != "pending" {
		return nil, contestdomain.Invalid("invalid frozen submission visibility")
	}
	switch input.Rule {
	case "":
		input.Rule = contestdomain.FormatICPC
	case "acm":
		input.Rule = contestdomain.FormatICPC
	case contestdomain.FormatICPC, contestdomain.FormatIOI, contestdomain.FormatOI:
	default:
		return nil, contestdomain.Invalid("rule must be icpc, ioi or oi")
	}
	if input.Visibility == "" {
		input.Visibility = "public"
	}
	if input.Visibility != "public" && input.Visibility != "private" && input.Visibility != "password" {
		return nil, contestdomain.Invalid("invalid visibility")
	}
	if input.Feedback == "" {
		// OI contests are scored on the final submission, so live feedback
		// would change what contestants can do; default them to silent.
		if input.Rule == contestdomain.FormatOI {
			input.Feedback = contestdomain.FeedbackNone
		} else {
			input.Feedback = contestdomain.FeedbackFull
		}
	}
	switch input.Feedback {
	case contestdomain.FeedbackFull, contestdomain.FeedbackSummary, contestdomain.FeedbackNone:
	default:
		return nil, contestdomain.Invalid("feedback must be full, summary or none")
	}
	if input.PenaltyMinutes == 0 && input.Rule == contestdomain.FormatICPC {
		input.PenaltyMinutes = 20
	}
	if input.PenaltyMinutes < 0 || input.PenaltyMinutes > 1440 {
		return nil, contestdomain.Invalid("penalty minutes must be between 0 and 1440")
	}
	if !input.EndAt.After(input.BeginAt) {
		return nil, contestdomain.Invalid("end time must be after begin time")
	}
	if input.FreezeAt != nil && (!input.FreezeAt.After(input.BeginAt) || !input.FreezeAt.Before(input.EndAt)) {
		return nil, contestdomain.Invalid("freeze time must be within contest time")
	}
	if input.UnfreezeAt != nil {
		if input.FreezeAt == nil {
			return nil, contestdomain.Invalid("unfreeze time requires a freeze time")
		}
		if input.UnfreezeAt.Before(*input.FreezeAt) {
			return nil, contestdomain.Invalid("unfreeze time must not be before the freeze time")
		}
	}

	passwordHash := ""
	if input.Visibility == "password" {
		if input.Password == "" {
			if existingPasswordHash == "" {
				return nil, contestdomain.Invalid("password required")
			}
			// Empty means preserve the current hash under the store's row lock.
			// Reusing this pre-lock snapshot would overwrite a concurrent rotation.
		} else {
			var err error
			passwordHash, err = passwords.HashPassword(input.Password)
			if err != nil {
				return nil, err
			}
		}
	}

	return &contestdomain.PersistInput{
		Admission:             input.Admission,
		AllowSelfRegistration: input.AllowSelfRegistration,
		AllowLateRegistration: input.AllowLateRegistration,
		Title:                 input.Title, Description: input.Description, Rule: input.Rule,
		BeginAt: input.BeginAt, EndAt: input.EndAt,
		FreezeAt: input.FreezeAt, UnfreezeAt: input.UnfreezeAt,
		PenaltyMinutes: input.PenaltyMinutes, PenalizeCompileError: input.PenalizeCompileError,
		Feedback: input.Feedback, Visibility: input.Visibility, PasswordHash: passwordHash,
		RankboardVisible: input.RankboardVisible, ShowProblemMetadata: input.ShowProblemMetadata, SubmissionVisibility: input.SubmissionVisibility, SourceCodeVisibility: input.SourceCodeVisibility, FrozenSubmissionVisibility: input.FrozenSubmissionVisibility,
	}, nil
}
