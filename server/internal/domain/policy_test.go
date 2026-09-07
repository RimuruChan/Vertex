package domain_test

import (
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/domain/dto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domain permission boundaries", func() {
	var scope domain.Scope
	BeforeEach(func() {
		owner := "owner"
		scope = domain.Scope{Domain: domain.Domain{ID: "a", Slug: "classroom", OwnerID: &owner, Visibility: "private"}, UserID: "member", MemberStatus: "active"}
	})
	It("separates public discovery, membership and capabilities", func() {
		scope.UserID = ""
		scope.MemberStatus = ""
		scope.Domain.Visibility = "public"
		Expect(scope.CanDiscover()).To(BeTrue())
		Expect(scope.CanEnter()).To(BeTrue())
		Expect(scope.Permissions()).To(BeEmpty())
		scope.Domain.Visibility = "private"
		Expect(scope.CanDiscover()).To(BeFalse())
		scope.UserID = "member"
		scope.MemberStatus = "invited"
		Expect(scope.CanDiscover()).To(BeTrue())
		Expect(scope.CanEnter()).To(BeFalse())
		Expect(scope.Allows(domain.CreateSubmission)).To(BeFalse())
	})
	It("keeps domain ownership outside editable roles", func() {
		scope.UserID = "owner"
		scope.MemberRole = "viewer"
		Expect(scope.IsOwner()).To(BeTrue())
		Expect(scope.Allows(domain.ManageRoles)).To(BeTrue())
		Expect(scope.CanGovernOwnership()).To(BeTrue())
		scope.Domain.Official = true
		Expect(scope.CanGovernOwnership()).To(BeFalse())
	})
	It("keeps restoration capability visible after archiving without granting transfer", func() {
		scope.UserID = "owner"
		scope.Domain.Archived = true
		response := dto.FromDomain(scope)
		Expect(response.CanArchive).To(BeTrue())
		Expect(response.CanTransfer).To(BeFalse())
		Expect(response.Permissions).To(BeEmpty())
		scope.Domain.Official = true
		Expect(dto.FromDomain(scope).CanArchive).To(BeFalse())
	})
	It("gates inherited group abilities on domain state", func() {
		group := domain.Group{DomainID: "a", OwnerID: "someone", ViewerRole: "manager"}
		Expect(scope.CanManageGroup(group)).To(BeTrue())
		scope.MemberStatus = "suspended"
		Expect(scope.CanManageGroup(group)).To(BeFalse())
		scope.MemberStatus = "active"
		group.DomainID = "b"
		Expect(scope.CanManageGroup(group)).To(BeFalse())
		group.DomainID = "a"
		scope.Domain.Archived = true
		Expect(scope.CanManageGroup(group)).To(BeFalse())
	})
	It("does not turn domain roles into site permissions", func() {
		scope.RolePermissions = []domain.Permission{domain.ManageMembers, domain.Permission("site.admin")}
		Expect(scope.Allows(domain.ManageMembers)).To(BeTrue())
		Expect(scope.Allows(domain.Permission("site.admin"))).To(BeFalse())
		scope.SiteAdmin = true
		Expect(scope.Allows(domain.Permission("arbitrary.unknown"))).To(BeFalse())
	})
})
