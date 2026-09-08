package domain_test

import (
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Contest capabilities", func() {
	member := func() tenancydomain.Scope {
		return tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public"}, UserID: "member", MemberStatus: "active", RolePermissions: []tenancydomain.Permission{tenancydomain.CreateSubmission}}
	}
	It("does not conflate editors, jury and observers", func() {
		scope := member()
		editor := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionMembers, contestdomain.Grants{Editor: true}, false)
		Expect(editor.Edit && editor.PreviewProblems).To(BeTrue())
		Expect(editor.ViewJury || editor.Rejudge || editor.Reply || editor.ManageAccess || editor.Submit).To(BeFalse())
		jury := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionMembers, contestdomain.Grants{Jury: true}, false)
		Expect(jury.ViewJury && jury.Rejudge && jury.Reply).To(BeTrue())
		Expect(jury.Edit || jury.ManageAccess || jury.Register).To(BeFalse())
		registeredJury := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionMembers, contestdomain.Grants{Jury: true}, true)
		Expect(registeredJury.Submit).To(BeFalse())
		observer := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionMembers, contestdomain.Grants{Observer: true}, true)
		Expect(observer.ViewJury && observer.PreviewProblems).To(BeTrue())
		Expect(observer.Edit || observer.Rejudge || observer.Reply || observer.Submit || observer.Register).To(BeFalse())
	})
	It("requires both current eligibility and registration for ordinary submissions", func() {
		scope := member()
		eligible := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionRestricted, contestdomain.Grants{Participant: true}, false)
		Expect(eligible.View && eligible.Register).To(BeTrue())
		Expect(eligible.Submit || eligible.PreviewProblems).To(BeFalse())
		registered := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionRestricted, contestdomain.Grants{Participant: true}, true)
		Expect(registered.Submit).To(BeTrue())
		revoked := contestdomain.EffectivePermissions(scope, "owner", "private", contestdomain.AdmissionRestricted, contestdomain.Grants{}, true)
		Expect(revoked.View || revoked.Submit).To(BeFalse())
	})
	It("removes all collaboration power from suspended members and makes archives read-only", func() {
		scope := member()
		scope.MemberStatus = "suspended"
		Expect(contestdomain.EffectivePermissions(scope, scope.UserID, "public", contestdomain.AdmissionMembers, contestdomain.Grants{Jury: true}, true)).To(Equal(contestdomain.Permissions{}))
		scope = member()
		scope.Domain.Archived = true
		caps := contestdomain.EffectivePermissions(scope, scope.UserID, "private", contestdomain.AdmissionMembers, contestdomain.Grants{}, false)
		Expect(caps.View && caps.PreviewProblems && caps.ViewJury).To(BeTrue())
		Expect(caps.Edit || caps.ManageAccess || caps.Rejudge || caps.Reply || caps.Submit).To(BeFalse())
	})
})
