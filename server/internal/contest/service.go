package contest

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/domain"
)

var (
	ErrInvalidInput        = errors.New("invalid contest input")
	ErrNotActive           = errors.New("contest is not active")
	ErrNotParticipant      = errors.New("user is not registered for contest")
	ErrProblemNotInContest = errors.New("problem is not in contest")
	ErrRegistrationClosed  = errors.New("contest already started")
	ErrInvalidPassword     = errors.New("invalid contest password")
	ErrRegistrationNeeded  = errors.New("contest registration required")
	ErrRankboardHidden     = errors.New("rankboard is hidden")
	ErrNotFound            = errors.New("contest not found")
	ErrForbidden           = errors.New("contest forbidden")
	ErrVersionConflict     = errors.New("contest problem version changed; refresh before switching")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

// UpsertInput is the jury-facing contest configuration.
type UpsertInput struct {
	Admission            string
	Title                string
	Description          string
	Rule                 string
	BeginAt              time.Time
	EndAt                time.Time
	FreezeAt             *time.Time
	UnfreezeAt           *time.Time
	PenaltyMinutes       int
	PenalizeCompileError bool
	Feedback             string
	Visibility           string
	Password             string
	RankboardVisible     bool
}

// PersistInput contains only values that may cross the persistence boundary.
// Plain-text contest passwords are deliberately excluded.
type PersistInput struct {
	Admission            string
	Title                string
	Description          string
	Rule                 string
	BeginAt              time.Time
	EndAt                time.Time
	FreezeAt             *time.Time
	UnfreezeAt           *time.Time
	PenaltyMinutes       int
	PenalizeCompileError bool
	Feedback             string
	Visibility           string
	PasswordHash         string
	RankboardVisible     bool
}

type Details struct {
	Contest  *Contest
	Problems []Problem
	// Staff is the caller's contest-scoped role, empty for a plain contestant.
	Staff string
}

type Repository interface {
	UseProblemVersion(ctx context.Context, contestID, problemID string, version, expected int) error
	Access(ctx context.Context, contestID, userID string) (Access, error)
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
	Delete(ctx context.Context, id string) error
	Create(ctx context.Context, createdBy string, input *PersistInput) (*Contest, error)
	Update(ctx context.Context, id string, input *PersistInput) (*Contest, error)
	List(ctx context.Context, limit, offset int, keyword ...string) ([]Contest, int, error)
	ListAdmin(ctx context.Context, limit, offset int, keyword ...string) ([]Contest, int, error)
	Get(ctx context.Context, id string) (*Contest, error)
	Problems(ctx context.Context, contestID string) ([]Problem, error)
	Problem(ctx context.Context, contestID, problemID string) (*ProblemDetail, error)
	SetProblems(ctx context.Context, contestID string, entries []ProblemEntry) error
	IsParticipant(ctx context.Context, contestID, userID string) (bool, error)
	Register(ctx context.Context, contestID, userID string, verifiedPasswordHash ...string) error
	HasProblem(ctx context.Context, contestID, problemID string) (bool, error)
	Rankboard(ctx context.Context, contestID string, jury bool) (*Rankboard, error)
	StaffRole(ctx context.Context, contestID, userID string) (string, error)
	ListStaff(ctx context.Context, contestID string) ([]Staff, error)
	AddStaff(ctx context.Context, contestID, username, role string) (*Staff, error)
	RemoveStaff(ctx context.Context, contestID, userID string) error
}

func (s *Service) UseProblemVersion(ctx context.Context, contestID, problemID string, version, expected int) error {
	if version <= 0 || expected <= 0 {
		return invalid("positive version and expected version are required")
	}
	access, err := s.repository.Access(ctx, contestID, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Rejudge {
		return ErrForbidden
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
	repository     Repository
	clarifications ClarificationRepository
	passwords      PasswordManager
	now            func() time.Time
}

// NewService wires the contest domain. The clarification repository is
// optional so a deployment can run without the question channel; every
// clarification entry point degrades to "not found" when it is absent.
func NewService(repository Repository, passwords PasswordManager) *Service {
	service := &Service{repository: repository, passwords: passwords, now: time.Now}
	if clarifications, ok := repository.(ClarificationRepository); ok {
		service.clarifications = clarifications
	}
	return service
}

func (s *Service) List(ctx context.Context, limit, offset int, admin bool, keyword ...string) ([]Contest, int, error) {
	if len(keyword) > 0 && len(keyword[0]) > 200 {
		return nil, 0, invalid("search keyword must be at most 200 bytes")
	}
	if admin {
		return s.repository.ListAdmin(ctx, limit, offset, keyword...)
	}
	return s.repository.List(ctx, limit, offset, keyword...)
}

// Viewer resolves the caller's contest-scoped rights once, so every other
// entry point can reason about a single value instead of re-deriving roles.
func (s *Service) Viewer(ctx context.Context, contestID, userID, _ string) (Viewer, error) {
	access, err := s.repository.Access(ctx, contestID, userID)
	if err != nil {
		return Viewer{}, err
	}
	viewer := Viewer{UserID: userID, Access: &access, Role: "user"}
	if access.Scope.SiteAdmin {
		viewer.Role = "admin"
	}
	if access.Permissions.Rejudge {
		viewer.Staff = StaffJury
	} else if access.Permissions.ViewJury {
		viewer.Staff = StaffObserver
	}
	return viewer, nil
}

func (s *Service) Details(ctx context.Context, id, userID, role string, _ bool) (*Details, error) {
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
		visible := make([]Problem, 0, len(problems))
		for _, problem := range problems {
			if problem.Visibility == "public" || (registered && !s.now().Before(item.BeginAt)) {
				visible = append(visible, problem)
			}
		}
		problems = visible
	}
	return &Details{Contest: item, Problems: problems, Staff: viewer.Staff}, nil
}

// Problem returns a full statement through the contest access boundary.
// Ordinary users must be registered and may only open it once the contest has
// started. Staff and the contest creator may preview it before the start.
func (s *Service) Problem(
	ctx context.Context, contestID, problemID, userID, role string,
) (*ProblemDetail, error) {
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
			return nil, ErrNotFound
		}
		if item.Visibility == "public" {
			registered, err = s.isParticipant(ctx, item.ID, userID)
			if err != nil {
				return nil, err
			}
		}
		if !registered {
			return nil, ErrRegistrationNeeded
		}
	}
	detail, err := s.repository.Problem(ctx, item.ID, problemID)
	if errors.Is(err, ErrProblemNotInContest) {
		return nil, ErrNotFound
	}
	return detail, err
}

// resolveViewerAccess applies the shared contest visibility boundary. It also
// resolves password-contest participation once for callers that need to
// decide whether protected content may be returned.
func (s *Service) resolveViewerAccess(
	ctx context.Context, item *Contest, userID, role string,
) (Viewer, bool, error) {
	viewer, err := s.Viewer(ctx, item.ID, userID, role)
	if err != nil {
		return viewer, false, err
	}
	if !viewer.Access.Permissions.View {
		return viewer, false, ErrNotFound
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
		return viewer, false, ErrNotFound
	}
	registered, err := s.isParticipant(ctx, item.ID, userID)
	return viewer, registered, err
}

func (s *Service) Create(ctx context.Context, createdBy string, input UpsertInput) (*Contest, error) {
	persisted, err := prepareInput(input, "", s.passwords)
	if err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, createdBy, persisted)
}

func (s *Service) Update(ctx context.Context, id string, input UpsertInput) (*Contest, error) {
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
func (s *Service) SetProblems(ctx context.Context, contestID string, entries []ProblemEntry) error {
	prepared := make([]ProblemEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	labels := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		entry.ProblemID = strings.TrimSpace(entry.ProblemID)
		if entry.ProblemID == "" {
			return invalid("problem ID is required")
		}
		if _, duplicate := seen[entry.ProblemID]; duplicate {
			return invalid("a problem may only appear once in a contest")
		}
		seen[entry.ProblemID] = struct{}{}

		entry.Label = strings.TrimSpace(entry.Label)
		if entry.Label == "" {
			entry.Label = defaultLabel(index)
		}
		if !problemLabelPattern.MatchString(entry.Label) {
			return invalid("problem label must start with a letter and contain at most 8 letters or digits")
		}
		if _, duplicate := labels[entry.Label]; duplicate {
			return invalid("problem labels must be unique")
		}
		labels[entry.Label] = struct{}{}
		entry.Color = strings.TrimSpace(entry.Color)
		if len(entry.Color) > 32 {
			return invalid("problem colour must be at most 32 characters")
		}
		if entry.Points <= 0 {
			entry.Points = 100
		}
		if entry.Points > 100000 {
			return invalid("problem points must be at most 100000")
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
	viewer, err := s.Viewer(ctx, contestID, userID, role)
	if err != nil {
		return err
	}
	if !viewer.Access.Permissions.View {
		return ErrNotFound
	}
	if !viewer.Access.Permissions.Register {
		return ErrForbidden
	}
	if !s.now().Before(item.BeginAt) {
		return ErrRegistrationClosed
	}
	registered, err := s.repository.IsParticipant(ctx, item.ID, userID)
	if err != nil || registered {
		return err
	}
	if item.Visibility == "password" && !s.passwords.CheckPassword(item.PasswordHash, password) {
		return ErrInvalidPassword
	}
	return s.repository.Register(ctx, item.ID, userID, item.PasswordHash)
}

func (s *Service) Registration(ctx context.Context, contestID, userID, role string) (bool, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return false, err
	}
	viewer, err := s.Viewer(ctx, contestID, userID, role)
	if err != nil {
		return false, err
	}
	if !viewer.Access.Permissions.View {
		return false, ErrNotFound
	}
	return s.repository.IsParticipant(ctx, item.ID, userID)
}

// Rankboard returns the scoreboard for one viewer. Jury and observers always
// receive the unfrozen board; everyone else receives the frozen view while the
// freeze is in effect.
func (s *Service) Rankboard(ctx context.Context, contestID, userID, role string, juryView bool) (*Rankboard, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}
	viewer, err := s.Viewer(ctx, contestID, userID, role)
	if err != nil {
		return nil, err
	}
	if !viewer.Access.Permissions.View {
		return nil, ErrNotFound
	}
	if !viewer.IsStaff() {
		if s.now().Before(item.BeginAt) {
			return nil, ErrNotFound
		}
		if item.Visibility == "password" {
			registered, err := s.isParticipant(ctx, item.ID, userID)
			if err != nil {
				return nil, err
			}
			if !registered {
				return nil, ErrRegistrationNeeded
			}
		}
		if !item.RankboardVisible {
			return nil, ErrRankboardHidden
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
		return ErrNotActive
	}
	viewer, registered, err := s.resolveViewerAccess(ctx, item, userID, role)
	if err != nil {
		return err
	}
	if !viewer.Access.Permissions.Submit {
		return ErrNotParticipant
	}
	if !viewer.IsStaff() {
		if item.Visibility == "public" {
			registered, err = s.isParticipant(ctx, item.ID, userID)
			if err != nil {
				return err
			}
		}
		if !registered {
			return ErrNotParticipant
		}
	}
	hasProblem, err := s.repository.HasProblem(ctx, item.ID, problemID)
	if err != nil {
		return err
	}
	if !hasProblem {
		return ErrProblemNotInContest
	}
	return nil
}

// Feedback reports the feedback level that applies to a contestant's own
// submission in this contest right now. Staff always see everything.
func (s *Service) Feedback(ctx context.Context, contestID string, viewer Viewer) (string, error) {
	if viewer.IsStaff() {
		return FeedbackFull, nil
	}
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return FeedbackFull, err
	}
	return item.FeedbackFor(s.now()), nil
}

// ---------- staff ----------

func (s *Service) ListStaff(ctx context.Context, contestID string) ([]Staff, error) {
	if _, err := s.RequireStaff(ctx, contestID, domain.ActorID(ctx), ""); err != nil {
		return nil, err
	}
	return s.repository.ListStaff(ctx, contestID)
}

func (s *Service) AddStaff(ctx context.Context, contestID, username, role string) (*Staff, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, invalid("username is required")
	}
	if role != StaffJury && role != StaffObserver {
		return nil, invalid("staff role must be jury or observer")
	}
	return s.repository.AddStaff(ctx, contestID, username, role)
}

func (s *Service) RemoveStaff(ctx context.Context, contestID, userID string) error {
	return s.repository.RemoveStaff(ctx, contestID, userID)
}

// RequireJury resolves the viewer and rejects callers without jury rights.
func (s *Service) RequireJury(ctx context.Context, contestID, userID, role string) (Viewer, error) {
	viewer, err := s.Viewer(ctx, contestID, userID, role)
	if err != nil {
		return viewer, err
	}
	if !viewer.IsJury() {
		return viewer, ErrForbidden
	}
	return viewer, nil
}

// RequireStaff resolves the viewer and rejects callers without jury or
// observer rights.
func (s *Service) RequireStaff(ctx context.Context, contestID, userID, role string) (Viewer, error) {
	viewer, err := s.Viewer(ctx, contestID, userID, role)
	if err != nil {
		return viewer, err
	}
	if !viewer.IsStaff() {
		return viewer, ErrForbidden
	}
	return viewer, nil
}

func (s *Service) Get(ctx context.Context, id string) (*Contest, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) Grants(ctx context.Context, id string) ([]AccessGrant, error) {
	return s.repository.Grants(ctx, id)
}
func (s *Service) SetGrant(ctx context.Context, id string, input GrantInput) error {
	input.Username, input.Group = strings.TrimSpace(input.Username), strings.TrimSpace(input.Group)
	if (input.Username == "") == (input.Group == "") {
		return invalid("select exactly one domain member or group")
	}
	if !validAccessRole(input.Role) {
		return invalid("unknown contest access role")
	}
	return s.repository.SetGrant(ctx, id, input)
}
func (s *Service) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	if grantID <= 0 {
		return invalid("grant ID must be positive")
	}
	return s.repository.RemoveGrant(ctx, id, grantID)
}
func (s *Service) Transfer(ctx context.Context, id, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return invalid("new owner is required")
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
		return ErrForbidden
	}
	return nil
}

func (s *Service) isParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	return s.repository.IsParticipant(ctx, contestID, userID)
}

func prepareInput(input UpsertInput, existingPasswordHash string, passwords PasswordManager) (*PersistInput, error) {
	if input.Admission == "" {
		input.Admission = AdmissionMembers
	}
	if input.Admission != AdmissionMembers && input.Admission != AdmissionRestricted {
		return nil, invalid("admission must be members or restricted")
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, invalid("title required")
	}
	switch input.Rule {
	case "":
		input.Rule = FormatICPC
	case "acm":
		input.Rule = FormatICPC
	case FormatICPC, FormatIOI, FormatOI:
	default:
		return nil, invalid("rule must be icpc, ioi or oi")
	}
	if input.Visibility == "" {
		input.Visibility = "public"
	}
	if input.Visibility != "public" && input.Visibility != "private" && input.Visibility != "password" {
		return nil, invalid("invalid visibility")
	}
	if input.Feedback == "" {
		// OI contests are scored on the final submission, so live feedback
		// would change what contestants can do; default them to silent.
		if input.Rule == FormatOI {
			input.Feedback = FeedbackNone
		} else {
			input.Feedback = FeedbackFull
		}
	}
	switch input.Feedback {
	case FeedbackFull, FeedbackSummary, FeedbackNone:
	default:
		return nil, invalid("feedback must be full, summary or none")
	}
	if input.PenaltyMinutes == 0 && input.Rule == FormatICPC {
		input.PenaltyMinutes = 20
	}
	if input.PenaltyMinutes < 0 || input.PenaltyMinutes > 1440 {
		return nil, invalid("penalty minutes must be between 0 and 1440")
	}
	if !input.EndAt.After(input.BeginAt) {
		return nil, invalid("end time must be after begin time")
	}
	if input.FreezeAt != nil && (!input.FreezeAt.After(input.BeginAt) || !input.FreezeAt.Before(input.EndAt)) {
		return nil, invalid("freeze time must be within contest time")
	}
	if input.UnfreezeAt != nil {
		if input.FreezeAt == nil {
			return nil, invalid("unfreeze time requires a freeze time")
		}
		if input.UnfreezeAt.Before(*input.FreezeAt) {
			return nil, invalid("unfreeze time must not be before the freeze time")
		}
	}

	passwordHash := ""
	if input.Visibility == "password" {
		if input.Password == "" {
			if existingPasswordHash == "" {
				return nil, invalid("password required")
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

	return &PersistInput{
		Admission: input.Admission,
		Title:     input.Title, Description: input.Description, Rule: input.Rule,
		BeginAt: input.BeginAt, EndAt: input.EndAt,
		FreezeAt: input.FreezeAt, UnfreezeAt: input.UnfreezeAt,
		PenaltyMinutes: input.PenaltyMinutes, PenalizeCompileError: input.PenalizeCompileError,
		Feedback: input.Feedback, Visibility: input.Visibility, PasswordHash: passwordHash,
		RankboardVisible: input.RankboardVisible,
	}, nil
}

func invalid(message string) error { return &ValidationError{Message: message} }
