package domain

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Service owns domain use cases. The store's write boundary supplies a freshly
// locked scope so authorization and persistence cannot be separated by a race.
type Service struct{ store *Store }

func NewService(store *Store) *Service { return &Service{store: store} }

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

func (s *Service) List(ctx context.Context, userID string, filter Filters) ([]Scope, int, error) {
	return s.store.List(ctx, userID, filter)
}
func (s *Service) Get(ctx context.Context, slug, userID string) (Scope, error) {
	if !slugPattern.MatchString(slug) {
		return Scope{}, ErrNotFound
	}
	scope, err := s.store.Scope(ctx, slug, userID)
	if err != nil {
		return Scope{}, err
	}
	if !scope.CanDiscover() {
		return Scope{}, ErrNotFound
	}
	return scope, nil
}
func (s *Service) memberScope(ctx context.Context, slug, userID string) (Scope, error) {
	if userID == "" {
		return Scope{}, ErrUnauthenticated
	}
	scope, err := s.Get(ctx, slug, userID)
	if err != nil {
		return Scope{}, err
	}
	if !scope.SiteAdmin && !scope.ActiveMember() {
		return Scope{}, ErrForbidden
	}
	return scope, nil
}
func (s *Service) Create(ctx context.Context, userID string, input CreateInput) (Scope, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	if input.Visibility == "" {
		input.Visibility = "private"
	}
	if input.JoinPolicy == "" {
		input.JoinPolicy = "invite"
	}
	if !slugPattern.MatchString(input.Slug) || input.Slug == OfficialSlug || !validName(input.Name) || len(input.Description) > 8000 || !validVisibility(input.Visibility) || !validJoinPolicy(input.JoinPolicy) {
		return Scope{}, invalid("域标识、名称、可见性或加入策略不合法")
	}
	return s.store.create(ctx, userID, input)
}
func (s *Service) Update(ctx context.Context, slug, userID string, input UpdateInput) (Scope, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || len(input.Description) > 8000 || !validVisibility(input.Visibility) || !validJoinPolicy(input.JoinPolicy) {
		return Scope{}, invalid("域设置不合法")
	}
	err := s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.Allows(ManageSettings) {
			return ErrForbidden
		}
		if tx.scope.Domain.Official && (input.Visibility != "public" || input.JoinPolicy != "open") {
			return ErrForbidden
		}
		if err := tx.updateDomain(input); err != nil {
			return err
		}
		return tx.audit("domain.updated", tx.scope.Domain.ID)
	})
	if err != nil {
		return Scope{}, err
	}
	return s.Get(ctx, slug, userID)
}
func (s *Service) Archive(ctx context.Context, slug, userID string, archived bool) error {
	return s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.CanGovernOwnership() {
			return ErrForbidden
		}
		if err := tx.archiveDomain(archived); err != nil {
			return err
		}
		return tx.audit("domain.archive_changed", tx.scope.Domain.ID)
	})
}
func (s *Service) Transfer(ctx context.Context, slug, userID, username string) error {
	return s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.CanGovernOwnership() || tx.scope.Domain.Archived {
			return ErrForbidden
		}
		member, err := tx.member(strings.TrimSpace(username))
		if err != nil {
			return err
		}
		if member.Status != "active" {
			return invalid("新所有者必须是有效域成员")
		}
		if err = tx.transferDomain(member.UserID); err != nil {
			return err
		}
		return tx.audit("domain.owner_transferred", member.UserID)
	})
}
func (s *Service) Join(ctx context.Context, slug, userID string) (Scope, error) {
	err := s.store.write(ctx, slug, userID, func(tx *session) error {
		if tx.scope.Domain.Archived || tx.scope.MemberStatus == "suspended" {
			return ErrForbidden
		}
		if tx.scope.ActiveMember() {
			return nil
		}
		member, err := tx.member(tx.actor.Username)
		if err != nil {
			return err
		}
		if member.Status == "invited" {
			member.Status = "active"
		} else {
			if tx.scope.Domain.JoinPolicy == "invite" {
				return ErrForbidden
			}
			member.RoleKey = "member"
			member.Status = "active"
			if tx.scope.Domain.JoinPolicy == "approval" {
				member.Status = "pending"
			}
		}
		if err = tx.saveMember(member); err != nil {
			return err
		}
		return tx.audit("member.joined_or_requested", member.UserID)
	})
	if err != nil {
		return Scope{}, err
	}
	return s.Get(ctx, slug, userID)
}
func (s *Service) Members(ctx context.Context, slug, userID string, filter Filters) ([]Member, int, error) {
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return nil, 0, err
	}
	return s.store.members(ctx, scope.Domain.ID, filter)
}
func (s *Service) SetMember(ctx context.Context, slug, userID string, input MemberInput) error {
	if input.Username == "" || !rolePattern.MatchString(input.RoleKey) || !slices.Contains([]string{"active", "pending", "invited", "suspended"}, input.Status) {
		return invalid("成员、角色或状态不合法")
	}
	return s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.Allows(ManageMembers) {
			return ErrForbidden
		}
		member, err := tx.member(input.Username)
		if err != nil {
			return err
		}
		if tx.scope.Domain.OwnerID != nil && member.UserID == *tx.scope.Domain.OwnerID && (member.RoleKey != input.RoleKey || input.Status != "active") {
			return ErrConflict
		}
		role, err := tx.role(input.RoleKey)
		if err != nil {
			return err
		}
		if err = validateDelegation(tx.scope, role.Permissions); err != nil {
			return err
		}
		member.RoleKey, member.Status = input.RoleKey, input.Status
		if err = tx.saveMember(member); err != nil {
			return err
		}
		return tx.audit("member.updated", member.UserID)
	})
}
func (s *Service) Roles(ctx context.Context, slug, userID string) ([]Role, error) {
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return nil, err
	}
	return s.store.roles(ctx, scope.Domain.ID)
}
func (s *Service) SaveRole(ctx context.Context, slug, userID string, input RoleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if !rolePattern.MatchString(input.Key) || !validName(input.Name) {
		return invalid("角色标识或名称不合法")
	}
	for _, p := range input.Permissions {
		if !knownPermission(p) {
			return invalid("不能授予未知或站点级权限")
		}
	}
	input.Permissions = slices.Clone(input.Permissions)
	slices.Sort(input.Permissions)
	input.Permissions = slices.Compact(input.Permissions)
	if input.Permissions == nil {
		input.Permissions = []Permission{}
	}
	return s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.Allows(ManageRoles) {
			return ErrForbidden
		}
		existing, err := tx.role(input.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if existing.Builtin {
			return ErrForbidden
		}
		if err = validateDelegation(tx.scope, input.Permissions); err != nil {
			return err
		}
		if err = tx.saveRole(input); err != nil {
			return err
		}
		return tx.audit("role.saved", input.Key)
	})
}
func (s *Service) DeleteRole(ctx context.Context, slug, userID, key string) error {
	return s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.Allows(ManageRoles) {
			return ErrForbidden
		}
		role, err := tx.role(key)
		if err != nil {
			return err
		}
		if role.Builtin {
			return ErrForbidden
		}
		if err = tx.removeRole(key); err != nil {
			return err
		}
		return tx.audit("role.deleted", key)
	})
}

func (s *Service) Groups(ctx context.Context, slug, userID string, filter Filters) ([]Group, int, error) {
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return nil, 0, err
	}
	return s.store.groups(ctx, scope, filter)
}
func (s *Service) Group(ctx context.Context, slug, userID, ref string) (Group, Scope, error) {
	if !validGroupRef(ref) {
		return Group{}, Scope{}, ErrNotFound
	}
	scope, err := s.memberScope(ctx, slug, userID)
	if err != nil {
		return Group{}, Scope{}, err
	}
	group, err := readGroup(ctx, s.store.db.Pool, scope, ref)
	return group, scope, err
}
func (s *Service) CreateGroup(ctx context.Context, slug, userID string, input GroupInput) (Group, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || len(input.Description) > 8000 {
		return Group{}, invalid("群组设置不合法")
	}
	var result Group
	err := s.store.write(ctx, slug, userID, func(tx *session) error {
		if !tx.scope.Allows(ManageGroups) {
			return ErrForbidden
		}
		if input.OwnerUsername == "" {
			input.OwnerUsername = tx.actor.Username
		}
		owner, err := tx.member(input.OwnerUsername)
		if err != nil {
			return err
		}
		if owner.Status != "active" {
			return invalid("组所有者必须是有效域成员")
		}
		id, err := tx.createGroup(input, owner.UserID)
		if err != nil {
			return err
		}
		result, err = tx.group(id)
		if err != nil {
			return err
		}
		return tx.audit("group.created", id)
	})
	return result, err
}
func (s *Service) UpdateGroup(ctx context.Context, slug, userID, ref string, input GroupInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || len(input.Description) > 8000 {
		return invalid("群组设置不合法")
	}
	return s.mutateGroup(ctx, slug, userID, ref, func(tx *session, group Group) error {
		if err := tx.updateGroup(group.ID, input); err != nil {
			return err
		}
		return tx.audit("group.updated", group.ID)
	})
}
func (s *Service) mutateGroup(ctx context.Context, slug, userID, ref string, fn func(*session, Group) error) error {
	if !validGroupRef(ref) {
		return ErrNotFound
	}
	return s.store.write(ctx, slug, userID, func(tx *session) error {
		group, err := tx.group(ref)
		if err != nil {
			return err
		}
		if !tx.scope.CanManageGroup(group) {
			return ErrForbidden
		}
		return fn(tx, group)
	})
}
func (s *Service) DeleteGroup(ctx context.Context, slug, userID, ref string) error {
	return s.mutateGroup(ctx, slug, userID, ref, func(tx *session, group Group) error {
		if !tx.scope.Allows(ManageGroups) && group.OwnerID != userID {
			return ErrForbidden
		}
		if err := tx.deleteGroup(group.ID); err != nil {
			return err
		}
		return tx.audit("group.deleted", group.ID)
	})
}
func (s *Service) TransferGroup(ctx context.Context, slug, userID, ref, username string) error {
	return s.mutateGroup(ctx, slug, userID, ref, func(tx *session, group Group) error {
		if !tx.scope.Allows(ManageGroups) && group.OwnerID != userID {
			return ErrForbidden
		}
		member, err := tx.member(username)
		if err != nil {
			return err
		}
		if member.Status != "active" {
			return invalid("新所有者必须是有效域成员")
		}
		if err = tx.transferGroup(group.ID, member.UserID); err != nil {
			return err
		}
		return tx.audit("group.owner_transferred", group.ID)
	})
}
func (s *Service) GroupMembers(ctx context.Context, slug, userID, ref string, filter Filters) ([]GroupMember, int, error) {
	group, scope, err := s.Group(ctx, slug, userID, ref)
	if err != nil {
		return nil, 0, err
	}
	if !scope.CanManageGroup(group) && group.ViewerRole == "" {
		return nil, 0, ErrForbidden
	}
	return s.store.groupMembers(ctx, scope.Domain.ID, group.ID, filter)
}
func (s *Service) SetGroupMember(ctx context.Context, slug, userID, ref, username, role string, remove bool) error {
	if !remove && role != "member" && role != "manager" {
		return invalid("组内角色不合法")
	}
	return s.mutateGroup(ctx, slug, userID, ref, func(tx *session, group Group) error {
		member, err := tx.lookupMember(username, !remove)
		if err != nil {
			return err
		}
		if (!remove && member.Status != "active") || member.Status == "" {
			return invalid("只能添加有效的同域成员")
		}
		if member.UserID == group.OwnerID && (remove || role != "manager") {
			return ErrConflict
		}
		if err = tx.setGroupMember(group.ID, member.UserID, role, remove); err != nil {
			return err
		}
		return tx.audit("group.member_changed", group.ID+":"+member.UserID)
	})
}
