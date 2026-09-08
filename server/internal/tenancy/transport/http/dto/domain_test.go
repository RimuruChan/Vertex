package dto

import (
	"testing"

	domain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

func TestArchivedDomainCanBeRestoredButNotTransferred(t *testing.T) {
	owner := "owner"
	scope := domain.Scope{Domain: domain.Domain{ID: "a", OwnerID: &owner, Archived: true}, UserID: owner, MemberStatus: "active"}
	response := FromDomain(scope)
	if !response.CanArchive || response.CanTransfer || len(response.Permissions) != 0 {
		t.Fatalf("incorrect archived-domain actions: %+v", response)
	}
	scope.Domain.Official = true
	if FromDomain(scope).CanArchive {
		t.Fatal("official domain offered an archive action")
	}
}
