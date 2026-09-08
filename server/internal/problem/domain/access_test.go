package domain_test

import (
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var _ = Describe("Problem capabilities", func() {
	member := func() tenancydomain.Scope {
		return tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID, Visibility: "public"}, UserID: "viewer", MemberStatus: "active"}
	}
	It("keeps public statements separate from private package access", func() {
		scope := tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public"}}
		public := problemdomain.EffectivePermissions(scope, "owner", "public", "")
		Expect(public.View).To(BeTrue())
		Expect(public.ReadPackage).To(BeFalse())
		Expect(public.Edit).To(BeFalse())
		Expect(problemdomain.EffectivePermissions(scope, "owner", "private", "").View).To(BeFalse())
	})
	It("allows readers to review and editors to edit without owner-only powers", func() {
		for _, role := range []problemdomain.AccessRole{problemdomain.AccessReader, problemdomain.AccessEditor} {
			caps := problemdomain.EffectivePermissions(member(), "owner", "draft", role)
			Expect(caps.ReadPackage).To(BeTrue())
			Expect(caps.View).To(BeTrue())
			Expect(caps.Edit).To(Equal(role == problemdomain.AccessEditor))
			Expect(caps.Publish || caps.ManageAccess || caps.Transfer || caps.Delete).To(BeFalse())
		}
	})
	It("retains ownership when creation permission is removed, but not when membership is suspended", func() {
		scope := member()
		Expect(scope.Allows(tenancydomain.CreateProblem)).To(BeFalse())
		caps := problemdomain.EffectivePermissions(scope, scope.UserID, "draft", problemdomain.AccessOwner)
		Expect(caps.Edit && caps.Publish && caps.Transfer && caps.ManageAccess && caps.Delete).To(BeTrue())
		scope.MemberStatus = "suspended"
		Expect(problemdomain.EffectivePermissions(scope, scope.UserID, "public", problemdomain.AccessOwner)).To(Equal(problemdomain.Permissions{}))
	})
	It("makes archives read-only for owners, collaborators and site administrators", func() {
		for _, role := range []problemdomain.AccessRole{problemdomain.AccessReader, problemdomain.AccessEditor, problemdomain.AccessOwner} {
			scope := member()
			scope.Domain.Archived = true
			owner := "owner"
			if role == problemdomain.AccessOwner {
				owner = scope.UserID
			}
			caps := problemdomain.EffectivePermissions(scope, owner, "draft", role)
			Expect(caps.ReadPackage).To(BeTrue())
			Expect(caps.Edit || caps.Publish || caps.ManageAccess || caps.Delete || caps.Transfer).To(BeFalse())
		}
		scope := member()
		scope.SiteAdmin, scope.Domain.Archived = true, true
		caps := problemdomain.EffectivePermissions(scope, "owner", "draft", "")
		Expect(caps.ReadPackage).To(BeTrue())
		Expect(caps.Edit || caps.Publish || caps.Delete || caps.Transfer).To(BeFalse())
	})
})

func TestAccess(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Problem Access") }
