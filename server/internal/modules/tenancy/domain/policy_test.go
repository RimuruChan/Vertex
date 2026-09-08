package domain_test

import (
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var _ = Describe("Domain permission boundaries", func() {
	var scope tenancydomain.Scope
	BeforeEach(func() {
		owner := "owner"
		scope = tenancydomain.Scope{Domain: tenancydomain.Domain{ID: "a", Slug: "classroom", OwnerID: &owner, Visibility: "private"}, UserID: "member", MemberStatus: "active"}
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
		Expect(scope.Allows(tenancydomain.CreateSubmission)).To(BeFalse())
	})
	It("keeps domain ownership outside editable roles", func() {
		scope.UserID = "owner"
		scope.MemberRole = "viewer"
		Expect(scope.IsOwner()).To(BeTrue())
		Expect(scope.Allows(tenancydomain.ManageRoles)).To(BeTrue())
		Expect(scope.CanGovernOwnership()).To(BeTrue())
		scope.Domain.Official = true
		Expect(scope.CanGovernOwnership()).To(BeFalse())
	})

	It("gates inherited group abilities on domain state", func() {
		group := tenancydomain.Group{DomainID: "a", OwnerID: "someone", ViewerRole: "manager"}
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
		scope.RolePermissions = []tenancydomain.Permission{tenancydomain.ManageMembers, tenancydomain.Permission("site.admin")}
		Expect(scope.Allows(tenancydomain.ManageMembers)).To(BeTrue())
		Expect(scope.Allows(tenancydomain.Permission("site.admin"))).To(BeFalse())
		scope.SiteAdmin = true
		Expect(scope.Allows(tenancydomain.Permission("arbitrary.unknown"))).To(BeFalse())
	})
})

func TestPolicy(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Tenancy policy") }
