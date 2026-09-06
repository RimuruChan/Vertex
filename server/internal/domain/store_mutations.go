package domain

import (
	"encoding/json"
)

func (tx *session) role(key string) (Role, error) {
	return readRole(tx.ctx, tx.tx, tx.scope.Domain.ID, key)
}
func (tx *session) group(ref string) (Group, error) { return readGroup(tx.ctx, tx.tx, tx.scope, ref) }

func (tx *session) updateDomain(input UpdateInput) error {
	_, err := tx.tx.ExecContext(tx.ctx, "UPDATE domains SET name=$2,description=$3,visibility=$4,join_policy=$5,updated_at=now() WHERE id=$1", tx.scope.Domain.ID, input.Name, input.Description, input.Visibility, input.JoinPolicy)
	return err
}
func (tx *session) archiveDomain(archived bool) error {
	_, err := tx.tx.ExecContext(tx.ctx, "UPDATE domains SET archived=$2,updated_at=now() WHERE id=$1", tx.scope.Domain.ID, archived)
	return err
}
func (tx *session) transferDomain(userID string) error {
	_, err := tx.tx.ExecContext(tx.ctx, "UPDATE domain_members SET role_key='admin',updated_at=now() WHERE domain_id=$1 AND user_id=$2", tx.scope.Domain.ID, userID)
	if err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(tx.ctx, "UPDATE domains SET owner_id=$2,updated_at=now() WHERE id=$1", tx.scope.Domain.ID, userID)
	return err
}
func (tx *session) saveRole(input RoleInput) error {
	permissions, err := json.Marshal(input.Permissions)
	if err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(tx.ctx, `INSERT INTO domain_roles(domain_id,key,name,permissions) VALUES($1,$2,$3,$4)
        ON CONFLICT(domain_id,key) DO UPDATE SET name=EXCLUDED.name,permissions=EXCLUDED.permissions WHERE NOT domain_roles.builtin`, tx.scope.Domain.ID, input.Key, input.Name, permissions)
	return err
}
func (tx *session) removeRole(key string) error {
	_, err := tx.tx.ExecContext(tx.ctx, "DELETE FROM domain_roles WHERE domain_id=$1 AND key=$2 AND NOT builtin", tx.scope.Domain.ID, key)
	return err
}
func (tx *session) createGroup(input GroupInput, ownerID string) (string, error) {
	var id string
	err := tx.tx.QueryRowxContext(tx.ctx, "INSERT INTO domain_groups(domain_id,name,description,owner_id) VALUES($1,$2,$3,$4) RETURNING id", tx.scope.Domain.ID, input.Name, input.Description, ownerID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, tx.setGroupMember(id, ownerID, "manager", false)
}
func (tx *session) updateGroup(id string, input GroupInput) error {
	_, err := tx.tx.ExecContext(tx.ctx, "UPDATE domain_groups SET name=$3,description=$4,updated_at=now() WHERE domain_id=$1 AND id=$2", tx.scope.Domain.ID, id, input.Name, input.Description)
	return err
}
func (tx *session) transferGroup(id, userID string) error {
	if err := tx.setGroupMember(id, userID, "manager", false); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(tx.ctx, "UPDATE domain_groups SET owner_id=$3,updated_at=now() WHERE domain_id=$1 AND id=$2", tx.scope.Domain.ID, id, userID)
	return err
}
func (tx *session) setGroupMember(groupID, userID, role string, remove bool) error {
	if remove {
		_, err := tx.tx.ExecContext(tx.ctx, "DELETE FROM domain_group_members WHERE domain_id=$1 AND group_id=$2 AND user_id=$3", tx.scope.Domain.ID, groupID, userID)
		return err
	}
	_, err := tx.tx.ExecContext(tx.ctx, `INSERT INTO domain_group_members(domain_id,group_id,user_id,role) VALUES($1,$2,$3,$4)
        ON CONFLICT(group_id,user_id) DO UPDATE SET role=EXCLUDED.role`, tx.scope.Domain.ID, groupID, userID, role)
	return err
}
func (tx *session) deleteGroup(id string) error {
	_, err := tx.tx.ExecContext(tx.ctx, "DELETE FROM domain_groups WHERE domain_id=$1 AND id=$2", tx.scope.Domain.ID, id)
	return err
}
