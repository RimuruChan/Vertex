package problem_test

import (
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Problem capabilities", func() {
	member := func() domain.Scope {
		return domain.Scope{Domain: domain.Domain{ID: domain.OfficialID, Visibility: "public"}, UserID: "viewer", MemberStatus: "active"}
	}
	It("keeps public statements separate from private package access", func() {
		scope := domain.Scope{Domain: domain.Domain{Visibility: "public"}}
		public := problem.EffectivePermissions(scope, "owner", "public", "")
		Expect(public.View).To(BeTrue())
		Expect(public.ReadPackage).To(BeFalse())
		Expect(public.Edit).To(BeFalse())
		Expect(problem.EffectivePermissions(scope, "owner", "private", "").View).To(BeFalse())
	})
	It("allows readers to review and editors to edit without owner-only powers", func() {
		for _, role := range []problem.AccessRole{problem.AccessReader, problem.AccessEditor} {
			caps := problem.EffectivePermissions(member(), "owner", "draft", role)
			Expect(caps.ReadPackage).To(BeTrue())
			Expect(caps.View).To(BeTrue())
			Expect(caps.Edit).To(Equal(role == problem.AccessEditor))
			Expect(caps.Publish || caps.ManageAccess || caps.Transfer || caps.Delete).To(BeFalse())
		}
	})
	It("retains ownership when creation permission is removed, but not when membership is suspended", func() {
		scope := member()
		Expect(scope.Allows(domain.CreateProblem)).To(BeFalse())
		caps := problem.EffectivePermissions(scope, scope.UserID, "draft", problem.AccessOwner)
		Expect(caps.Edit && caps.Publish && caps.Transfer && caps.ManageAccess && caps.Delete).To(BeTrue())
		scope.MemberStatus = "suspended"
		Expect(problem.EffectivePermissions(scope, scope.UserID, "public", problem.AccessOwner)).To(Equal(problem.Permissions{}))
	})
	It("makes archives read-only for owners, collaborators and site administrators", func() {
		for _, role := range []problem.AccessRole{problem.AccessReader, problem.AccessEditor, problem.AccessOwner} {
			scope := member()
			scope.Domain.Archived = true
			owner := "owner"
			if role == problem.AccessOwner {
				owner = scope.UserID
			}
			caps := problem.EffectivePermissions(scope, owner, "draft", role)
			Expect(caps.ReadPackage).To(BeTrue())
			Expect(caps.Edit || caps.Publish || caps.ManageAccess || caps.Delete || caps.Transfer).To(BeFalse())
		}
		scope := member()
		scope.SiteAdmin, scope.Domain.Archived = true, true
		caps := problem.EffectivePermissions(scope, "owner", "draft", "")
		Expect(caps.ReadPackage).To(BeTrue())
		Expect(caps.Edit || caps.Publish || caps.Delete || caps.Transfer).To(BeFalse())
	})
})
