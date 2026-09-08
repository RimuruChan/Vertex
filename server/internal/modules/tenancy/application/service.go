package application

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

// Service owns domain use cases. The store's write boundary supplies a freshly
// locked scope so authorization and persistence cannot be separated by a race.
type Service struct{ repository tenancydomain.Repository }

func NewService(repository tenancydomain.Repository) *Service {
	return &Service{repository: repository}
}

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
var rolePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validName(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) > 0 && utf8.RuneCountInString(value) <= 100
}
func validVisibility(value string) bool { return value == "public" || value == "private" }
func validJoinPolicy(value string) bool {
	return value == "open" || value == "approval" || value == "invite"
}
func validGroupRef(ref string) bool {
	value, err := strconv.ParseInt(ref, 10, 64)
	return (err == nil && value > 0) || uuidPattern.MatchString(ref)
}

func (s *Service) List(ctx context.Context, userID string, filter tenancydomain.Filters) ([]tenancydomain.Scope, int, error) {
	return s.repository.List(ctx, userID, filter)
}
func (s *Service) Get(ctx context.Context, slug, userID string) (tenancydomain.Scope, error) {
	if !slugPattern.MatchString(slug) {
		return tenancydomain.Scope{}, tenancydomain.ErrNotFound
	}
	scope, err := s.repository.Scope(ctx, slug, userID)
	if err != nil {
		return tenancydomain.Scope{}, err
	}
	if !scope.CanDiscover() {
		return tenancydomain.Scope{}, tenancydomain.ErrNotFound
	}
	return scope, nil
}
func (s *Service) memberScope(ctx context.Context, slug, userID string) (tenancydomain.Scope, error) {
	if userID == "" {
		return tenancydomain.Scope{}, tenancydomain.ErrUnauthenticated
	}
	scope, err := s.Get(ctx, slug, userID)
	if err != nil {
		return tenancydomain.Scope{}, err
	}
	if !scope.SiteAdmin && !scope.ActiveMember() {
		return tenancydomain.Scope{}, tenancydomain.ErrForbidden
	}
	return scope, nil
}
func (s *Service) Create(ctx context.Context, userID string, input tenancydomain.CreateInput) (tenancydomain.Scope, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	if input.Visibility == "" {
		input.Visibility = "private"
	}
	if input.JoinPolicy == "" {
		input.JoinPolicy = "invite"
	}
	if !slugPattern.MatchString(input.Slug) || input.Slug == tenancydomain.OfficialSlug || !validName(input.Name) || len(input.Description) > 8000 || !validVisibility(input.Visibility) || !validJoinPolicy(input.JoinPolicy) {
		return tenancydomain.Scope{}, tenancydomain.Invalid("域标识、名称、可见性或加入策略不合法")
	}
	return s.repository.Create(ctx, userID, input)
}
func (s *Service) Update(ctx context.Context, slug, userID string, input tenancydomain.UpdateInput) (tenancydomain.Scope, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || len(input.Description) > 8000 || !validVisibility(input.Visibility) || !validJoinPolicy(input.JoinPolicy) {
		return tenancydomain.Scope{}, tenancydomain.Invalid("域设置不合法")
	}
	err := s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().Allows(tenancydomain.ManageSettings) {
			return tenancydomain.ErrForbidden
		}
		if tx.Scope().Domain.Official && (input.Visibility != "public" || input.JoinPolicy != "open") {
			return tenancydomain.ErrForbidden
		}
		if err := tx.UpdateDomain(input); err != nil {
			return err
		}
		return tx.Audit("domain.updated", tx.Scope().Domain.ID)
	})
	if err != nil {
		return tenancydomain.Scope{}, err
	}
	return s.Get(ctx, slug, userID)
}
func (s *Service) Archive(ctx context.Context, slug, userID string, archived bool) error {
	return s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().CanGovernOwnership() {
			return tenancydomain.ErrForbidden
		}
		if err := tx.ArchiveDomain(archived); err != nil {
			return err
		}
		return tx.Audit("domain.archive_changed", tx.Scope().Domain.ID)
	})
}
func (s *Service) Transfer(ctx context.Context, slug, userID, username string) error {
	return s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().CanGovernOwnership() || tx.Scope().Domain.Archived {
			return tenancydomain.ErrForbidden
		}
		member, err := tx.Member(strings.TrimSpace(username))
		if err != nil {
			return err
		}
		if member.Status != "active" {
			return tenancydomain.Invalid("新所有者必须是有效域成员")
		}
		if err = tx.TransferDomain(member.UserID); err != nil {
			return err
		}
		return tx.Audit("domain.owner_transferred", member.UserID)
	})
}
func (s *Service) Join(ctx context.Context, slug, userID string) (tenancydomain.Scope, error) {
	err := s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if tx.Scope().Domain.Archived || tx.Scope().MemberStatus == "suspended" {
			return tenancydomain.ErrForbidden
		}
		if tx.Scope().ActiveMember() {
			return nil
		}
		member, err := tx.Member(tx.ActorUsername())
		if err != nil {
			return err
		}
		if member.Status == "invited" {
			member.Status = "active"
		} else {
			if tx.Scope().Domain.JoinPolicy == "invite" {
				return tenancydomain.ErrForbidden
			}
			member.RoleKey = "member"
			member.Status = "active"
			if tx.Scope().Domain.JoinPolicy == "approval" {
				member.Status = "pending"
			}
		}
		if err = tx.SaveMember(member); err != nil {
			return err
		}
		return tx.Audit("member.joined_or_requested", member.UserID)
	})
	if err != nil {
		return tenancydomain.Scope{}, err
	}
	return s.Get(ctx, slug, userID)
}
func (s *Service) Members(ctx context.Context, slug, userID string, filter tenancydomain.Filters) ([]tenancydomain.Member, int, error) {
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.Members(ctx, scope.Domain.ID, filter)
}
func (s *Service) SetMember(ctx context.Context, slug, userID string, input tenancydomain.MemberInput) error {
	if input.Username == "" || !rolePattern.MatchString(input.RoleKey) || !slices.Contains([]string{"active", "pending", "invited", "suspended"}, input.Status) {
		return tenancydomain.Invalid("成员、角色或状态不合法")
	}
	return s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().Allows(tenancydomain.ManageMembers) {
			return tenancydomain.ErrForbidden
		}
		member, err := tx.Member(input.Username)
		if err != nil {
			return err
		}
		if tx.Scope().Domain.OwnerID != nil && member.UserID == *tx.Scope().Domain.OwnerID && (member.RoleKey != input.RoleKey || input.Status != "active") {
			return tenancydomain.ErrConflict
		}
		role, err := tx.Role(input.RoleKey)
		if err != nil {
			return err
		}
		if err = tenancydomain.ValidateDelegation(tx.Scope(), role.Permissions); err != nil {
			return err
		}
		member.RoleKey, member.Status = input.RoleKey, input.Status
		if err = tx.SaveMember(member); err != nil {
			return err
		}
		return tx.Audit("member.updated", member.UserID)
	})
}
func (s *Service) Roles(ctx context.Context, slug, userID string) ([]tenancydomain.Role, error) {
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return nil, err
	}
	return s.repository.Roles(ctx, scope.Domain.ID)
}
func (s *Service) SaveRole(ctx context.Context, slug, userID string, input tenancydomain.RoleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if !rolePattern.MatchString(input.Key) || !validName(input.Name) {
		return tenancydomain.Invalid("角色标识或名称不合法")
	}
	for _, p := range input.Permissions {
		if !tenancydomain.KnownPermission(p) {
			return tenancydomain.Invalid("不能授予未知或站点级权限")
		}
	}
	input.Permissions = slices.Clone(input.Permissions)
	slices.Sort(input.Permissions)
	input.Permissions = slices.Compact(input.Permissions)
	if input.Permissions == nil {
		input.Permissions = []tenancydomain.Permission{}
	}
	return s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().Allows(tenancydomain.ManageRoles) {
			return tenancydomain.ErrForbidden
		}
		existing, err := tx.Role(input.Key)
		if err != nil && !errors.Is(err, tenancydomain.ErrNotFound) {
			return err
		}
		if existing.Builtin {
			return tenancydomain.ErrForbidden
		}
		if err = tenancydomain.ValidateDelegation(tx.Scope(), input.Permissions); err != nil {
			return err
		}
		if err = tx.SaveRole(input); err != nil {
			return err
		}
		return tx.Audit("role.saved", input.Key)
	})
}
func (s *Service) DeleteRole(ctx context.Context, slug, userID, key string) error {
	return s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().Allows(tenancydomain.ManageRoles) {
			return tenancydomain.ErrForbidden
		}
		role, err := tx.Role(key)
		if err != nil {
			return err
		}
		if role.Builtin {
			return tenancydomain.ErrForbidden
		}
		if err = tx.RemoveRole(key); err != nil {
			return err
		}
		return tx.Audit("role.deleted", key)
	})
}

func (s *Service) Groups(ctx context.Context, slug, userID string, filter tenancydomain.Filters) ([]tenancydomain.Group, int, error) {
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return nil, 0, err
	}
	return s.repository.Groups(ctx, scope, filter)
}
func (s *Service) Group(ctx context.Context, slug, userID, ref string) (tenancydomain.Group, tenancydomain.Scope, error) {
	if !validGroupRef(ref) {
		return tenancydomain.Group{}, tenancydomain.Scope{}, tenancydomain.ErrNotFound
	}
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return tenancydomain.Group{}, tenancydomain.Scope{}, err
	}
	group, err := s.repository.Group(ctx, scope, ref)
	return group, scope, err
}
func (s *Service) CreateGroup(ctx context.Context, slug, userID string, input tenancydomain.GroupInput) (tenancydomain.Group, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || len(input.Description) > 8000 {
		return tenancydomain.Group{}, tenancydomain.Invalid("群组设置不合法")
	}
	var result tenancydomain.Group
	err := s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		if !tx.Scope().Allows(tenancydomain.ManageGroups) {
			return tenancydomain.ErrForbidden
		}
		if input.OwnerUsername == "" {
			input.OwnerUsername = tx.ActorUsername()
		}
		owner, err := tx.Member(input.OwnerUsername)
		if err != nil {
			return err
		}
		if owner.Status != "active" {
			return tenancydomain.Invalid("组所有者必须是有效域成员")
		}
		id, err := tx.CreateGroup(input, owner.UserID)
		if err != nil {
			return err
		}
		result, err = tx.Group(id)
		if err != nil {
			return err
		}
		return tx.Audit("group.created", id)
	})
	return result, err
}
func (s *Service) UpdateGroup(ctx context.Context, slug, userID, ref string, input tenancydomain.GroupInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || len(input.Description) > 8000 {
		return tenancydomain.Invalid("群组设置不合法")
	}
	return s.mutateGroup(ctx, slug, userID, ref, func(tx tenancydomain.Governance, group tenancydomain.Group) error {
		if err := tx.UpdateGroup(group.ID, input); err != nil {
			return err
		}
		return tx.Audit("group.updated", group.ID)
	})
}
func (s *Service) mutateGroup(ctx context.Context, slug, userID, ref string, fn func(tenancydomain.Governance, tenancydomain.Group) error) error {
	if !validGroupRef(ref) {
		return tenancydomain.ErrNotFound
	}
	return s.repository.WithinGovernance(ctx, slug, userID, func(tx tenancydomain.Governance) error {
		group, err := tx.Group(ref)
		if err != nil {
			return err
		}
		if !tx.Scope().CanManageGroup(group) {
			return tenancydomain.ErrForbidden
		}
		return fn(tx, group)
	})
}
func (s *Service) DeleteGroup(ctx context.Context, slug, userID, ref string) error {
	return s.mutateGroup(ctx, slug, userID, ref, func(tx tenancydomain.Governance, group tenancydomain.Group) error {
		if !tx.Scope().Allows(tenancydomain.ManageGroups) && group.OwnerID != userID {
			return tenancydomain.ErrForbidden
		}
		if err := tx.DeleteGroup(group.ID); err != nil {
			return err
		}
		return tx.Audit("group.deleted", group.ID)
	})
}
func (s *Service) TransferGroup(ctx context.Context, slug, userID, ref, username string) error {
	return s.mutateGroup(ctx, slug, userID, ref, func(tx tenancydomain.Governance, group tenancydomain.Group) error {
		if !tx.Scope().Allows(tenancydomain.ManageGroups) && group.OwnerID != userID {
			return tenancydomain.ErrForbidden
		}
		member, err := tx.Member(username)
		if err != nil {
			return err
		}
		if member.Status != "active" {
			return tenancydomain.Invalid("新所有者必须是有效域成员")
		}
		if err = tx.TransferGroup(group.ID, member.UserID); err != nil {
			return err
		}
		return tx.Audit("group.owner_transferred", group.ID)
	})
}
func (s *Service) GroupMembers(ctx context.Context, slug, userID, ref string, filter tenancydomain.Filters) ([]tenancydomain.GroupMember, int, error) {
	group, scope, err := s.Group(ctx, slug, userID, ref)
	if err != nil {
		return nil, 0, err
	}
	if !scope.CanManageGroup(group) && group.ViewerRole == "" {
		return nil, 0, tenancydomain.ErrForbidden
	}
	return s.repository.GroupMembers(ctx, scope.Domain.ID, group.ID, filter)
}
func (s *Service) SetGroupMember(ctx context.Context, slug, userID, ref, username, role string, remove bool) error {
	if !remove && role != "member" && role != "manager" {
		return tenancydomain.Invalid("组内角色不合法")
	}
	return s.mutateGroup(ctx, slug, userID, ref, func(tx tenancydomain.Governance, group tenancydomain.Group) error {
		member, err := tx.LookupMember(username, !remove)
		if err != nil {
			return err
		}
		if (!remove && member.Status != "active") || member.Status == "" {
			return tenancydomain.Invalid("只能添加有效的同域成员")
		}
		if member.UserID == group.OwnerID && (remove || role != "manager") {
			return tenancydomain.ErrConflict
		}
		if err = tx.SetGroupMember(group.ID, member.UserID, role, remove); err != nil {
			return err
		}
		return tx.Audit("group.member_changed", group.ID+":"+member.UserID)
	})
}
