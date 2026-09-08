package domain

import "context"

// Repository exposes domain governance and its read models without leaking a
// database connection or transaction into the application layer.
type Repository interface {
	Scope(ctx context.Context, slug, actorID string) (Scope, error)
	List(ctx context.Context, actorID string, filters Filters) ([]Scope, int, error)
	Create(ctx context.Context, actorID string, input CreateInput) (Scope, error)
	Members(ctx context.Context, domainID string, filters Filters) ([]Member, int, error)
	Roles(ctx context.Context, domainID string) ([]Role, error)
	Groups(ctx context.Context, scope Scope, filters Filters) ([]Group, int, error)
	Group(ctx context.Context, scope Scope, ref string) (Group, error)
	GroupMembers(ctx context.Context, domainID, groupID string, filters Filters) ([]GroupMember, int, error)
	WithinGovernance(ctx context.Context, slug, actorID string, operation func(Governance) error) error
}

// Governance is a transaction-scoped domain repository. Its scope and actor
// are reloaded under the account/domain locks before the callback starts.
// Every operation, including the audit record, commits or rolls back together.
// A Governance value is valid only for the duration of WithinGovernance.
type Governance interface {
	Scope() Scope
	ActorUsername() string
	Member(username string) (Member, error)
	LookupMember(username string, activeAccountOnly bool) (Member, error)
	Role(key string) (Role, error)
	Group(ref string) (Group, error)
	UpdateDomain(input UpdateInput) error
	ArchiveDomain(archived bool) error
	TransferDomain(userID string) error
	SaveMember(member Member) error
	SaveRole(input RoleInput) error
	RemoveRole(key string) error
	CreateGroup(input GroupInput, ownerID string) (string, error)
	UpdateGroup(id string, input GroupInput) error
	TransferGroup(id, userID string) error
	SetGroupMember(groupID, userID, role string, remove bool) error
	DeleteGroup(id string) error
	Audit(action, target string) error
}
