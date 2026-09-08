package domain

import (
	"time"

	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

const (
	AccessEditor        = "editor"
	AccessJury          = "jury"
	AccessObserver      = "observer"
	AccessParticipant   = "participant"
	AdmissionMembers    = "members"
	AdmissionRestricted = "restricted"
)

type Grants struct{ Editor, Jury, Observer, Participant bool }

type Permissions struct {
	View, Edit, ManageAccess, Delete, Transfer bool
	PreviewProblems, ViewJury, Rejudge, Reply  bool
	Eligible, Register, Submit                 bool
}

type Access struct {
	BeginAt, EndAt                               time.Time
	PasswordHash                                 string
	Scope                                        tenancydomain.Scope
	ContestID, OwnerID, Visibility, Admission    string
	Grants                                       Grants
	Registered                                   bool
	AllowSelfRegistration, AllowLateRegistration bool
	Permissions                                  Permissions
}

// EffectivePermissions keeps preparation, jury operations and participation
// separate. Jury/owners may make non-ranking test submissions without gaining
// a participant row. Observers and editors do not get submission rights.
func EffectivePermissions(scope tenancydomain.Scope, ownerID, visibility, admission string, grants Grants, registered bool) Permissions {
	if !scope.CanEnter() {
		return Permissions{}
	}
	readScope := scope
	readScope.Domain.Archived = false
	governor := readScope.Allows(tenancydomain.ManageResources)
	owner := scope.ActiveMember() && scope.UserID == ownerID
	if !scope.ActiveMember() {
		grants = Grants{}
	}
	manager := governor || owner
	preview := manager || grants.Editor || grants.Jury || grants.Observer
	jury := manager || grants.Jury
	readJury := jury || grants.Observer
	eligible := readScope.Allows(tenancydomain.CreateSubmission) && (admission == AdmissionMembers || (admission == AdmissionRestricted && grants.Participant))
	canCompete := eligible && !preview
	writable := !scope.Domain.Archived
	return Permissions{
		View:         visibility == "public" || visibility == "password" || preview || grants.Participant || (registered && eligible),
		Edit:         writable && (manager || grants.Editor),
		ManageAccess: writable && manager, Delete: writable && manager, Transfer: writable && manager,
		PreviewProblems: preview, ViewJury: readJury,
		Rejudge: writable && jury, Reply: writable && jury,
		Eligible: eligible, Register: writable && canCompete,
		Submit: writable && scope.Allows(tenancydomain.CreateSubmission) && ((jury && !registered) || (canCompete && registered)),
	}
}

type AccessGrant struct {
	ID                                   int64
	UserID, Username, GroupID, GroupName *string
	Role                                 string
}

type GrantInput struct{ Username, Group, Role string }

func ValidAccessRole(role string) bool {
	return role == AccessEditor || role == AccessJury || role == AccessObserver || role == AccessParticipant
}
