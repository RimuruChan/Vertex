package contest_test

import (
	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Contest capabilities", func() {
	member := func() domain.Scope {
		return domain.Scope{Domain: domain.Domain{Visibility: "public"}, UserID: "member", MemberStatus: "active", RolePermissions: []domain.Permission{domain.CreateSubmission}}
	}
	It("does not conflate editors, jury and observers", func() {
		scope := member()
		editor := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionMembers, contest.Grants{Editor: true}, false)
		Expect(editor.Edit && editor.PreviewProblems).To(BeTrue())
		Expect(editor.ViewJury || editor.Rejudge || editor.Reply || editor.ManageAccess || editor.Submit).To(BeFalse())
		jury := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionMembers, contest.Grants{Jury: true}, false)
		Expect(jury.ViewJury && jury.Rejudge && jury.Reply).To(BeTrue())
		Expect(jury.Edit || jury.ManageAccess || jury.Register).To(BeFalse())
		registeredJury := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionMembers, contest.Grants{Jury: true}, true)
		Expect(registeredJury.Submit).To(BeFalse())
		observer := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionMembers, contest.Grants{Observer: true}, true)
		Expect(observer.ViewJury && observer.PreviewProblems).To(BeTrue())
		Expect(observer.Edit || observer.Rejudge || observer.Reply || observer.Submit || observer.Register).To(BeFalse())
	})
	It("requires both current eligibility and registration for ordinary submissions", func() {
		scope := member()
		eligible := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionRestricted, contest.Grants{Participant: true}, false)
		Expect(eligible.View && eligible.Register).To(BeTrue())
		Expect(eligible.Submit || eligible.PreviewProblems).To(BeFalse())
		registered := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionRestricted, contest.Grants{Participant: true}, true)
		Expect(registered.Submit).To(BeTrue())
		revoked := contest.EffectivePermissions(scope, "owner", "private", contest.AdmissionRestricted, contest.Grants{}, true)
		Expect(revoked.View || revoked.Submit).To(BeFalse())
	})
	It("removes all collaboration power from suspended members and makes archives read-only", func() {
		scope := member()
		scope.MemberStatus = "suspended"
		Expect(contest.EffectivePermissions(scope, scope.UserID, "public", contest.AdmissionMembers, contest.Grants{Jury: true}, true)).To(Equal(contest.Permissions{}))
		scope = member()
		scope.Domain.Archived = true
		caps := contest.EffectivePermissions(scope, scope.UserID, "private", contest.AdmissionMembers, contest.Grants{}, false)
		Expect(caps.View && caps.PreviewProblems && caps.ViewJury).To(BeTrue())
		Expect(caps.Edit || caps.ManageAccess || caps.Rejudge || caps.Reply || caps.Submit).To(BeFalse())
	})
})
