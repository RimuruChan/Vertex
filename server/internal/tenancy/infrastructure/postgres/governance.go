package postgres

import (
	"context"
	"encoding/json"

	domain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres/internal/dbgen"
)

// session is created only after account/domain locking in WithinGovernance.
// It exposes aggregate operations, never the underlying SQL transaction.
type session struct {
	ctx     context.Context
	queries *dbgen.Queries
	scope   domain.Scope
	actor   subject
}

func (s *session) Scope() domain.Scope   { return s.scope }
func (s *session) ActorUsername() string { return s.actor.Username }
func (s *session) Role(key string) (domain.Role, error) {
	return readRole(s.ctx, s.queries, s.scope.Domain.ID, key)
}
func (s *session) Group(ref string) (domain.Group, error) {
	return readGroup(s.ctx, s.queries, s.scope, ref)
}
func (s *session) Member(username string) (domain.Member, error) {
	return s.LookupMember(username, true)
}

func (s *session) LookupMember(username string, activeAccountOnly bool) (domain.Member, error) {
	row, err := s.queries.LookupDomainMember(s.ctx, dbgen.LookupDomainMemberParams{DomainID: s.scope.Domain.ID, Username: username, ActiveAccountOnly: activeAccountOnly})
	if err != nil {
		return domain.Member{}, notFound(err)
	}
	return domain.Member{UserID: row.UserID, Username: row.Username, RoleKey: row.RoleKey, Status: row.Status, JoinedAt: row.JoinedAt}, nil
}

func (s *session) Audit(action, target string) error {
	return s.queries.RecordDomainAudit(s.ctx, dbgen.RecordDomainAuditParams{DomainID: s.scope.Domain.ID, ActorID: s.actor.ID, Action: action, Target: target})
}

func (s *session) UpdateDomain(input domain.UpdateInput) error {
	return s.queries.UpdateDomain(s.ctx, dbgen.UpdateDomainParams{DomainID: s.scope.Domain.ID, Name: input.Name, Description: input.Description, Visibility: input.Visibility, JoinPolicy: input.JoinPolicy})
}

func (s *session) ArchiveDomain(archived bool) error {
	return s.queries.ArchiveDomain(s.ctx, dbgen.ArchiveDomainParams{DomainID: s.scope.Domain.ID, Archived: archived})
}

func (s *session) TransferDomain(userID string) error {
	if err := s.queries.PromoteNewDomainOwner(s.ctx, dbgen.PromoteNewDomainOwnerParams{DomainID: s.scope.Domain.ID, UserID: userID}); err != nil {
		return err
	}
	return s.queries.TransferDomainOwner(s.ctx, dbgen.TransferDomainOwnerParams{DomainID: s.scope.Domain.ID, OwnerID: userID})
}

func (s *session) SaveMember(member domain.Member) error {
	return s.queries.UpsertDomainMember(s.ctx, dbgen.UpsertDomainMemberParams{DomainID: s.scope.Domain.ID, UserID: member.UserID, RoleKey: member.RoleKey, Status: member.Status})
}

func (s *session) SaveRole(input domain.RoleInput) error {
	permissions, err := json.Marshal(input.Permissions)
	if err != nil {
		return err
	}
	return s.queries.UpsertDomainRole(s.ctx, dbgen.UpsertDomainRoleParams{DomainID: s.scope.Domain.ID, Key: input.Key, Name: input.Name, Permissions: permissions})
}

func (s *session) RemoveRole(key string) error {
	return s.queries.DeleteDomainRole(s.ctx, dbgen.DeleteDomainRoleParams{DomainID: s.scope.Domain.ID, Key: key})
}

func (s *session) CreateGroup(input domain.GroupInput, ownerID string) (string, error) {
	id, err := s.queries.CreateDomainGroup(s.ctx, dbgen.CreateDomainGroupParams{DomainID: s.scope.Domain.ID, Name: input.Name, Description: input.Description, OwnerID: ownerID})
	if err != nil {
		return "", err
	}
	return id, s.SetGroupMember(id, ownerID, "manager", false)
}

func (s *session) UpdateGroup(id string, input domain.GroupInput) error {
	return s.queries.UpdateDomainGroup(s.ctx, dbgen.UpdateDomainGroupParams{DomainID: s.scope.Domain.ID, GroupID: id, Name: input.Name, Description: input.Description})
}

func (s *session) TransferGroup(id, userID string) error {
	if err := s.SetGroupMember(id, userID, "manager", false); err != nil {
		return err
	}
	return s.queries.TransferGroupOwner(s.ctx, dbgen.TransferGroupOwnerParams{DomainID: s.scope.Domain.ID, GroupID: id, OwnerID: userID})
}

func (s *session) SetGroupMember(groupID, userID, role string, remove bool) error {
	if remove {
		return s.queries.DeleteGroupMember(s.ctx, dbgen.DeleteGroupMemberParams{DomainID: s.scope.Domain.ID, GroupID: groupID, UserID: userID})
	}
	return s.queries.UpsertGroupMember(s.ctx, dbgen.UpsertGroupMemberParams{DomainID: s.scope.Domain.ID, GroupID: groupID, UserID: userID, Role: role})
}

func (s *session) DeleteGroup(id string) error {
	return s.queries.DeleteDomainGroup(s.ctx, dbgen.DeleteDomainGroupParams{DomainID: s.scope.Domain.ID, GroupID: id})
}
